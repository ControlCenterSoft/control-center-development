package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresAuditSequenceOrderingConstraint(t *testing.T) {
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

	var triggerDefinition string
	var deferrable, initiallyDeferred bool
	if err := db.QueryRowContext(ctx, `SELECT pg_get_triggerdef(oid), tgdeferrable, tginitdeferred
FROM pg_trigger
WHERE tgrelid = 'cc_audit_events'::regclass
  AND tgname = 'cc_audit_events_sequence_ordering'
  AND NOT tgisinternal`).Scan(&triggerDefinition, &deferrable, &initiallyDeferred); err != nil {
		t.Fatalf("database must be migrated through 0013 audit sequence ordering: %v", err)
	}
	normalized := strings.ToUpper(triggerDefinition)
	if !strings.Contains(normalized, "AFTER INSERT") || !strings.Contains(normalized, "DEFERRABLE INITIALLY DEFERRED") || !deferrable || !initiallyDeferred {
		t.Fatalf("unexpected sequence-ordering trigger definition: %s deferrable=%v initially_deferred=%v", triggerDefinition, deferrable, initiallyDeferred)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('control-center:audit-chain'))`); err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var predecessorHash string
	var predecessorSequence int64
	err = tx.QueryRowContext(ctx, `SELECT hash, sequence_id FROM cc_audit_events ORDER BY sequence_id DESC LIMIT 1`).Scan(&predecessorHash, &predecessorSequence)
	if errors.Is(err, sql.ErrNoRows) {
		predecessorHash = auditSequenceOrderingHash("genesis:" + suffix)
		if err := tx.QueryRowContext(ctx, auditSequenceOrderingInsertSQL,
			auditSequenceOrderingUUID("genesis:"+suffix), nil, predecessorHash).Scan(&predecessorSequence); err != nil {
			t.Fatalf("insert genesis fixture: %v", err)
		}
	} else if err != nil {
		t.Fatalf("read audit chain head: %v", err)
	}

	var minimumSequence int64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(sequence_id) FROM cc_audit_events`).Scan(&minimumSequence); err != nil {
		t.Fatalf("read minimum audit sequence: %v", err)
	}
	if minimumSequence == -9223372036854775808 {
		t.Fatal("cannot allocate a lower sequence fixture")
	}
	badSequence := minimumSequence - 1

	if _, err := tx.ExecContext(ctx, `SAVEPOINT audit_sequence_ordering_bad`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, auditSequenceOrderingOverrideInsertSQL,
		badSequence, auditSequenceOrderingUUID("bad:"+suffix), predecessorHash, auditSequenceOrderingHash("bad:"+suffix)); err != nil {
		t.Fatalf("OVERRIDING SYSTEM VALUE fixture must reach deferred ordering constraint: %v", err)
	}
	_, orderingErr := tx.ExecContext(ctx, `SET CONSTRAINTS cc_audit_events_sequence_ordering IMMEDIATE`)
	if orderingErr == nil {
		t.Fatal("non-monotonic audit sequence unexpectedly passed deferred constraint")
	}
	if !strings.Contains(strings.ToLower(orderingErr.Error()), "sequence_id must advance predecessor") {
		t.Fatalf("non-monotonic sequence failed for an unexpected reason: %v", orderingErr)
	}
	if _, err := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT audit_sequence_ordering_bad`); err != nil {
		t.Fatalf("recover transaction after rejected sequence override: %v", err)
	}

	var childSequence int64
	if err := tx.QueryRowContext(ctx, auditSequenceOrderingInsertSQL,
		auditSequenceOrderingUUID("good:"+suffix), predecessorHash, auditSequenceOrderingHash("good:"+suffix)).Scan(&childSequence); err != nil {
		t.Fatalf("insert identity-generated child: %v", err)
	}
	if childSequence <= predecessorSequence {
		t.Fatalf("identity-generated child sequence did not advance predecessor: child=%d predecessor=%d", childSequence, predecessorSequence)
	}
	if _, err := tx.ExecContext(ctx, `SET CONSTRAINTS cc_audit_events_sequence_ordering IMMEDIATE`); err != nil {
		t.Fatalf("canonical identity-generated child rejected: %v", err)
	}
}

const auditSequenceOrderingInsertSQL = `INSERT INTO cc_audit_events
    (id, occurred_at, action, outcome, details, previous_hash, hash)
VALUES ($1, now(), 'audit.sequence_ordering_test', 'success', '{}'::jsonb, $2, $3)
RETURNING sequence_id`

const auditSequenceOrderingOverrideInsertSQL = `INSERT INTO cc_audit_events
    (sequence_id, id, occurred_at, action, outcome, details, previous_hash, hash)
OVERRIDING SYSTEM VALUE
VALUES ($1, $2, now(), 'audit.sequence_ordering_test', 'success', '{}'::jsonb, $3, $4)`

func auditSequenceOrderingHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func auditSequenceOrderingUUID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}
