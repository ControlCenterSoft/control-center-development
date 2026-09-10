package migrations

import (
	"strings"
	"testing"
)

func TestAuditLookupIndexMigrationIsScopedAndReversible(t *testing.T) {
	up := readMigration(t, "0010_audit_lookup_indexes.up.sql")
	for _, required := range []string{
		"CREATE INDEX IF NOT EXISTS cc_audit_events_correlation_sequence_idx",
		"ON cc_audit_events (correlation_id, sequence_id DESC)",
		"WHERE correlation_id IS NOT NULL",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0010 up migration lacks %q", required)
		}
	}
	upper := strings.ToUpper(up)
	for _, forbidden := range []string{"DROP TABLE", "ALTER TABLE", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("0010 lookup migration is not additive: found %q", forbidden)
		}
	}

	down := readMigration(t, "0010_audit_lookup_indexes.down.sql")
	if !strings.Contains(down, "DROP INDEX IF EXISTS cc_audit_events_correlation_sequence_idx") {
		t.Fatal("0010 down migration does not remove only its correlation lookup index")
	}
}
