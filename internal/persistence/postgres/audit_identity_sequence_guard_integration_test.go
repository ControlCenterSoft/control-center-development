package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresAuditIdentitySequenceGuard(t *testing.T) {
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
	if err := db.QueryRowContext(ctx, `SELECT pg_get_triggerdef(oid)
FROM pg_trigger
WHERE tgrelid = 'cc_audit_events'::regclass
  AND tgname = 'cc_audit_events_identity_sequence_guard'
  AND NOT tgisinternal`).Scan(&triggerDefinition); err != nil {
		t.Fatalf("database must be migrated through 0014 audit identity sequence guard: %v", err)
	}
	normalized := strings.ToUpper(triggerDefinition)
	if !strings.Contains(normalized, "BEFORE INSERT") || !strings.Contains(normalized, "FOR EACH ROW") {
		t.Fatalf("unexpected identity-sequence guard trigger definition: %s", triggerDefinition)
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
		predecessorHash = auditIdentitySequenceGuardHash("genesis:" + suffix)
		if err := tx.QueryRowContext(ctx, auditIdentitySequenceGuardInsertSQL,
			auditIdentitySequenceGuardUUID("genesis:"+suffix), nil, predecessorHash).Scan(&predecessorSequence); err != nil {
			t.Fatalf("insert genesis fixture: %v", err)
		}
	} else if err != nil {
		t.Fatalf("read audit chain head: %v", err)
	}

	var sequenceLast sql.NullInt64
	var increment, cacheSize int64
	var cycle bool
	if err := tx.QueryRowContext(ctx, `SELECT sequences.last_value, sequences.increment_by, sequences.cycle, sequences.cache_size
FROM pg_sequences AS sequences
JOIN pg_namespace AS namespace ON namespace.nspname = sequences.schemaname
JOIN pg_class AS relation ON relation.relnamespace = namespace.oid AND relation.relname = sequences.sequencename
WHERE relation.oid = pg_get_serial_sequence('cc_audit_events', 'sequence_id')::regclass`).Scan(&sequenceLast, &increment, &cycle, &cacheSize); err != nil {
		t.Fatalf("read audit identity sequence state: %v", err)
	}
	if !sequenceLast.Valid {
		t.Fatal("audit identity sequence must have an allocated value after an audit row exists")
	}
	if increment != 1 || cycle || cacheSize != 1 {
		t.Fatalf("audit identity sequence is not canonical: increment=%d cycle=%v cache=%d", increment, cycle, cacheSize)
	}
	if sequenceLast.Int64 < predecessorSequence {
		t.Fatalf("audit chain head is ahead of identity sequence: head=%d identity=%d", predecessorSequence, sequenceLast.Int64)
	}
	if sequenceLast.Int64 == math.MaxInt64 {
		t.Fatal("audit identity sequence is exhausted")
	}
	badSequence := sequenceLast.Int64 + 1

	if _, err := tx.ExecContext(ctx, `SAVEPOINT audit_identity_sequence_bad`); err != nil {
		t.Fatal(err)
	}
	_, guardErr := tx.ExecContext(ctx, auditIdentitySequenceGuardOverrideInsertSQL,
		badSequence,
		auditIdentitySequenceGuardUUID("bad:"+suffix),
		predecessorHash,
		auditIdentitySequenceGuardHash("bad:"+suffix),
	)
	if guardErr == nil {
		t.Fatal("sequence override ahead of identity state unexpectedly passed")
	}
	if !strings.Contains(strings.ToLower(guardErr.Error()), "must not advance beyond identity sequence state") {
		t.Fatalf("sequence override failed for an unexpected reason: %v", guardErr)
	}
	if _, err := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT audit_identity_sequence_bad`); err != nil {
		t.Fatalf("recover transaction after rejected sequence override: %v", err)
	}

	var childSequence int64
	if err := tx.QueryRowContext(ctx, auditIdentitySequenceGuardInsertSQL,
		auditIdentitySequenceGuardUUID("good:"+suffix), predecessorHash, auditIdentitySequenceGuardHash("good:"+suffix)).Scan(&childSequence); err != nil {
		t.Fatalf("insert identity-generated child: %v", err)
	}
	if childSequence <= predecessorSequence || childSequence <= sequenceLast.Int64 {
		t.Fatalf("identity-generated child did not advance chain and sequence state: child=%d predecessor=%d prior_identity=%d", childSequence, predecessorSequence, sequenceLast.Int64)
	}
	if _, err := tx.ExecContext(ctx, `SET CONSTRAINTS cc_audit_events_sequence_ordering IMMEDIATE`); err != nil {
		t.Fatalf("canonical identity-generated child rejected by sequence ordering: %v", err)
	}
}

const auditIdentitySequenceGuardInsertSQL = `INSERT INTO cc_audit_events
    (id, occurred_at, action, outcome, details, previous_hash, hash)
VALUES ($1, now(), 'audit.identity_sequence_guard_test', 'success', '{}'::jsonb, $2, $3)
RETURNING sequence_id`

const auditIdentitySequenceGuardOverrideInsertSQL = `INSERT INTO cc_audit_events
    (sequence_id, id, occurred_at, action, outcome, details, previous_hash, hash)
OVERRIDING SYSTEM VALUE
VALUES ($1, $2, now(), 'audit.identity_sequence_guard_test', 'success', '{}'::jsonb, $3, $4)`

func auditIdentitySequenceGuardHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func auditIdentitySequenceGuardUUID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}
