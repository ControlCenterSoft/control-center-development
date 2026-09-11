package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"control-center/internal/identity/auth"
	"control-center/internal/identity/bootstrap"
	"control-center/internal/identity/security"
	"control-center/internal/persistence/postgres"
)

type bootstrapCredentialUserStore struct {
	*postgres.IdentityStore
	credential bootstrap.CredentialFile
}

func (s *bootstrapCredentialUserStore) ChangePasswordAndRevokeSessions(ctx context.Context, id, expectedHash, newHash string, at time.Time) error {
	user, err := s.FindUserByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.IdentityStore.ChangePasswordAndRevokeSessions(ctx, id, expectedHash, newHash, at); err != nil {
		return err
	}
	if user.PasswordChangeRequired && strings.EqualFold(strings.TrimSpace(user.Username), "admin") {
		// The old bootstrap credential is no longer valid after the password
		// transaction commits. Cleanup is best-effort here; startup
		// reconciliation removes a stale file on the next process start.
		_ = s.credential.Remove()
	}
	return nil
}

func prepareBootstrapAdmin(ctx context.Context, db *sql.DB, store *postgres.IdentityStore, hasher security.PasswordHasher, credential bootstrap.CredentialFile, now time.Time) error {
	admin, err := store.FindUserByUsername(ctx, "admin")
	if err == nil {
		if !admin.PasswordChangeRequired {
			return credential.Remove()
		}
		return nil
	}
	if !errors.Is(err, auth.ErrNotFound) {
		return err
	}

	password, err := bootstrap.GeneratePassword()
	if err != nil {
		return err
	}
	passwordHash, err := hasher.HashBootstrapCredential(password)
	if err != nil {
		return err
	}

	credentialWritten := false
	_, _, err = postgres.BootstrapAdminWithCommitHook(ctx, db, "admin", passwordHash, now.UTC(), func() error {
		if err := credential.Write(password); err != nil {
			return err
		}
		credentialWritten = true
		return nil
	})
	if err != nil && credentialWritten {
		_ = credential.Remove()
	}
	return err
}
