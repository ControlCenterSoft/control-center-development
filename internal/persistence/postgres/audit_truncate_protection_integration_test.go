package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresAuditRejectsTruncate(t *testing.T) {
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
  AND tgname = 'cc_audit_events_no_truncate'
  AND NOT tgisinternal`).Scan(&triggerDefinition); err != nil {
		t.Fatalf("database must be migrated through 0012 audit truncate protection: %v", err)
	}
	normalized := strings.ToUpper(triggerDefinition)
	if !strings.Contains(normalized, "BEFORE TRUNCATE") || !strings.Contains(normalized, "EXECUTE FUNCTION CC_DENY_AUDIT_MUTATION()") {
		t.Fatalf("unexpected truncate trigger definition: %s", triggerDefinition)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	var before int64
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM cc_audit_events`).Scan(&before); err != nil {
		t.Fatal(err)
	}

	for _, statement := range []string{
		`TRUNCATE TABLE cc_audit_events`,
		`TRUNCATE TABLE cc_audit_events RESTART IDENTITY`,
	} {
		if _, err := tx.ExecContext(ctx, `SAVEPOINT audit_truncate_guard`); err != nil {
			t.Fatal(err)
		}
		_, truncateErr := tx.ExecContext(ctx, statement)
		if truncateErr == nil {
			t.Fatalf("%s unexpectedly succeeded", statement)
		}
		if !strings.Contains(strings.ToLower(truncateErr.Error()), "append-only") {
			t.Fatalf("%s failed for an unexpected reason: %v", statement, truncateErr)
		}
		if _, err := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT audit_truncate_guard`); err != nil {
			t.Fatalf("recover transaction after rejected truncate: %v", err)
		}
	}

	var after int64
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM cc_audit_events`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("rejected truncate changed audit row count: before=%d after=%d", before, after)
	}
}
