package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"control-center/internal/identity/auth"
	"control-center/internal/identity/bootstrap"
	identityapi "control-center/internal/identity/httpapi"
	"control-center/internal/identity/security"
	"control-center/internal/persistence/postgres"
)

func newIdentityHandler(environment string, db *sql.DB, sessionTTL, sessionIdleTimeout time.Duration) (*identityapi.Server, error) {
	ctx := context.Background()
	hasher := security.NewPasswordHasher()
	auditLog, err := postgres.NewAuditLog(db)
	if err != nil {
		return nil, err
	}
	if err := auditLog.VerifyChain(ctx); err != nil {
		return nil, err
	}
	store, err := postgres.NewIdentityStore(db)
	if err != nil {
		return nil, err
	}
	credentialStore := bootstrapCredentialStore(environment)
	if err := ensureBootstrapAdmin(ctx, db, store, hasher, credentialStore, time.Now().UTC()); err != nil {
		return nil, err
	}
	authService, err := auth.NewService(store, store, auditLog, hasher, sessionTTL, auth.WithSessionIdleTimeout(sessionIdleTimeout))
	if err != nil {
		return nil, err
	}
	authorizer := postgres.NewAuthorizer(db)

	return identityapi.NewServerWithSelfAccess(authService, authorizer, auditLog, identityapi.Config{
		InsecureCookiesForDevelopment: environment == "development" || environment == "test",
	})
}

func bootstrapCredentialStore(environment string) bootstrap.Store {
	if path := strings.TrimSpace(os.Getenv("CC_BOOTSTRAP_CREDENTIAL_PATH")); path != "" {
		return bootstrap.NewStore(path)
	}
	if environment == "development" || environment == "test" {
		return bootstrap.NewStore(filepath.Join(os.TempDir(), "control-center-bootstrap-password"))
	}
	return bootstrap.NewStore(bootstrap.DefaultCredentialPath)
}

func ensureBootstrapAdmin(ctx context.Context, db *sql.DB, identities *postgres.IdentityStore, hasher security.PasswordHasher, credentials bootstrap.Store, now time.Time) error {
	admin, err := identities.FindUserByUsername(ctx, "admin")
	if err == nil {
		// Upgrades never rotate an existing admin password. Once the first-login
		// requirement has been cleared, any leftover local bootstrap credential is
		// obsolete and is removed on startup as a fail-safe cleanup path.
		if !admin.PasswordChangeRequired {
			if err := credentials.Remove(); err != nil {
				return fmt.Errorf("remove obsolete bootstrap credential: %w", err)
			}
		}
		return nil
	}
	if !errors.Is(err, auth.ErrNotFound) {
		return fmt.Errorf("lookup bootstrap admin: %w", err)
	}

	secret, _, err := credentials.PrepareOrLoad(nil)
	if err != nil {
		return fmt.Errorf("prepare bootstrap credential: %w", err)
	}
	passwordHash, err := hasher.Hash(secret)
	if err != nil {
		return fmt.Errorf("hash bootstrap credential: %w", err)
	}
	_, created, err := postgres.BootstrapAdmin(ctx, db, "admin", passwordHash, now)
	if err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}
	if created {
		return nil
	}

	// A concurrent bootstrap may have won the insert race. Never overwrite that
	// account: only accept the race if its stored hash matches the exact local
	// one-time credential, otherwise fail closed.
	admin, err = identities.FindUserByUsername(ctx, "admin")
	if err != nil {
		return fmt.Errorf("reload concurrently bootstrapped admin: %w", err)
	}
	matches, err := hasher.Verify(secret, admin.PasswordHash)
	if err != nil {
		return fmt.Errorf("verify concurrent bootstrap credential: %w", err)
	}
	if !matches {
		return errors.New("bootstrap admin appeared with a different credential")
	}
	return nil
}
