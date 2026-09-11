package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// BootstrapAdminWithCommitHook is the generated-credential bootstrap boundary.
// The hook is invoked only for a newly-created administrator and immediately
// before the database transaction is committed. If the hook fails, the user and
// RBAC binding are rolled back so a clean install cannot become inaccessible
// because its one-time credential could not be persisted securely.
func BootstrapAdminWithCommitHook(ctx context.Context, db *sql.DB, username, passwordHash string, now time.Time, beforeCommit func() error) (string, bool, error) {
	if db == nil || strings.TrimSpace(username) == "" || passwordHash == "" || now.IsZero() {
		return "", false, errors.New("database, username, password hash, and time are required")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()

	userID := deterministicUUID("user:" + strings.ToLower(strings.TrimSpace(username)))
	var insertedID string
	err = tx.QueryRowContext(ctx, `INSERT INTO cc_local_users (id,username,display_name,password_hash,enabled,password_change_required,created_at,updated_at,password_changed_at) VALUES ($1::uuid,$2,'Control Center Administrator',$3,true,true,$4,$4,$4) ON CONFLICT DO NOTHING RETURNING id::text`, userID, strings.TrimSpace(username), passwordHash, now.UTC()).Scan(&insertedID)
	created := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	if !created {
		if err := tx.QueryRowContext(ctx, `SELECT id::text FROM cc_local_users WHERE lower(username)=lower($1)`, username).Scan(&userID); err != nil {
			return "", false, err
		}
		if err := tx.Commit(); err != nil {
			return "", false, err
		}
		return userID, false, nil
	}

	userID = insertedID
	bindingID := deterministicUUID("binding:" + userID + ":administrator:global")
	if _, err := tx.ExecContext(ctx, `INSERT INTO cc_rbac_user_bindings (id,user_id,role_name,scope_kind,scope_id,created_at,created_by) VALUES ($1::uuid,$2::uuid,'administrator','global',NULL,$3,$2::uuid) ON CONFLICT DO NOTHING`, bindingID, userID, now.UTC()); err != nil {
		return "", false, err
	}
	if beforeCommit != nil {
		if err := beforeCommit(); err != nil {
			return "", false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return userID, true, nil
}
