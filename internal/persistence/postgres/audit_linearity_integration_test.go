package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestPostgresAuditChainLinearityConstraints(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; PostgreSQL integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var constraintsReady bool
	if err := db.QueryRowContext(ctx, `SELECT
    to_regclass('cc_audit_events_previous_hash_uq') IS NOT NULL
    AND to_regclass('cc_audit_events_single_genesis_uq') IS NOT NULL
    AND EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='cc_audit_events'::regclass AND conname='cc_audit_events_hash_format')
    AND EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='cc_audit_events'::regclass AND conname='cc_audit_events_previous_hash_format')
    AND EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='cc_audit_events'::regclass AND conname='cc_audit_events_previous_hash_fk')`).Scan(&constraintsReady); err != nil || !constraintsReady {
		t.Fatalf("database must be migrated through 0011 audit chain linearity: ready=%v err=%v", constraintsReady, err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	t.Run("single genesis", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()

		var hasGenesis bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM cc_audit_events WHERE previous_hash IS NULL)`).Scan(&hasGenesis); err != nil {
			t.Fatal(err)
		}
		if !hasGenesis {
			if _, err := tx.ExecContext(ctx, auditConstraintInsertSQL, deterministicUUID("audit-linearity-genesis-a:"+suffix), nil, auditConstraintHash("genesis-a:"+suffix)); err != nil {
				t.Fatalf("insert first genesis fixture: %v", err)
			}
		}
		_, err = tx.ExecContext(ctx, auditConstraintInsertSQL, deterministicUUID("audit-linearity-genesis-b:"+suffix), nil, auditConstraintHash("genesis-b:"+suffix))
		if !isUniqueViolation(err) {
			t.Fatalf("second genesis must fail with unique violation: %v", err)
		}
	})

	t.Run("one child per predecessor", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('control-center:audit-chain'))`); err != nil {
			t.Fatal(err)
		}

		var predecessor string
		err = tx.QueryRowContext(ctx, `SELECT hash FROM cc_audit_events ORDER BY sequence_id DESC LIMIT 1`).Scan(&predecessor)
		if errors.Is(err, sql.ErrNoRows) {
			predecessor = auditConstraintHash("fork-genesis:" + suffix)
			if _, err := tx.ExecContext(ctx, auditConstraintInsertSQL, deterministicUUID("audit-linearity-fork-genesis:"+suffix), nil, predecessor); err != nil {
				t.Fatalf("insert fork genesis fixture: %v", err)
			}
		} else if err != nil {
			t.Fatalf("read audit chain head: %v", err)
		}

		if _, err := tx.ExecContext(ctx, auditConstraintInsertSQL, deterministicUUID("audit-linearity-child-a:"+suffix), predecessor, auditConstraintHash("child-a:"+suffix)); err != nil {
			t.Fatalf("insert first child fixture: %v", err)
		}
		_, err = tx.ExecContext(ctx, auditConstraintInsertSQL, deterministicUUID("audit-linearity-child-b:"+suffix), predecessor, auditConstraintHash("child-b:"+suffix))
		if !isUniqueViolation(err) {
			t.Fatalf("second child for one predecessor must fail with unique violation: %v", err)
		}
	})

	t.Run("predecessor must exist", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()

		missing := auditConstraintHash("missing-predecessor:" + suffix)
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM cc_audit_events WHERE hash=$1)`, missing).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatal("generated missing predecessor unexpectedly exists")
		}
		_, err = tx.ExecContext(ctx, auditConstraintInsertSQL, deterministicUUID("audit-linearity-orphan:"+suffix), missing, auditConstraintHash("orphan:"+suffix))
		if err == nil {
			t.Fatal("orphan audit predecessor was accepted")
		}
	})

	t.Run("hashes must be canonical lowercase sha256 hex", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('control-center:audit-chain'))`); err != nil {
			t.Fatal(err)
		}

		var predecessor string
		err = tx.QueryRowContext(ctx, `SELECT hash FROM cc_audit_events ORDER BY sequence_id DESC LIMIT 1`).Scan(&predecessor)
		if errors.Is(err, sql.ErrNoRows) {
			predecessor = auditConstraintHash("format-genesis:" + suffix)
			if _, insertErr := tx.ExecContext(ctx, auditConstraintInsertSQL, deterministicUUID("audit-linearity-format-genesis:"+suffix), nil, predecessor); insertErr != nil {
				t.Fatalf("insert format genesis fixture: %v", insertErr)
			}
		} else if err != nil {
			t.Fatalf("read audit chain head: %v", err)
		}
		_, err = tx.ExecContext(ctx, auditConstraintInsertSQL, deterministicUUID("audit-linearity-bad-hash:"+suffix), predecessor, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		if err == nil {
			t.Fatal("non-canonical uppercase audit hash was accepted")
		}
	})
}

const auditConstraintInsertSQL = `INSERT INTO cc_audit_events
    (id, occurred_at, action, outcome, details, previous_hash, hash)
VALUES ($1, now(), 'audit.chain_constraint_test', 'success', '{}'::jsonb, $2, $3)`

func auditConstraintHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
