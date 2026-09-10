package migrations

import (
	"strings"
	"testing"
)

func TestAuditSequenceOrderingMigrationContract(t *testing.T) {
	up := readMigration(t, "0013_audit_sequence_ordering.up.sql")
	for _, fragment := range []string{
		"WHERE child.sequence_id <= predecessor.sequence_id",
		"RAISE EXCEPTION 'cc_audit_events contains non-monotonic sequence/hash linkage'",
		"CREATE OR REPLACE FUNCTION cc_enforce_audit_sequence_ordering()",
		"IF NEW.sequence_id <= predecessor_sequence THEN",
		"CREATE CONSTRAINT TRIGGER cc_audit_events_sequence_ordering",
		"AFTER INSERT ON cc_audit_events",
		"DEFERRABLE INITIALLY DEFERRED",
		"FOR EACH ROW EXECUTE FUNCTION cc_enforce_audit_sequence_ordering()",
	} {
		if !strings.Contains(up, fragment) {
			t.Fatalf("0013 up migration lacks %q", fragment)
		}
	}

	upper := strings.ToUpper(up)
	for _, forbidden := range []string{
		"UPDATE CC_AUDIT_EVENTS",
		"DELETE FROM CC_AUDIT_EVENTS",
		"TRUNCATE CC_AUDIT_EVENTS",
		"DROP TABLE CC_AUDIT_EVENTS",
	} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("0013 up migration must not rewrite audit rows: found %q", forbidden)
		}
	}

	down := readMigration(t, "0013_audit_sequence_ordering.down.sql")
	for _, fragment := range []string{
		"DROP TRIGGER IF EXISTS cc_audit_events_sequence_ordering ON cc_audit_events",
		"DROP FUNCTION IF EXISTS cc_enforce_audit_sequence_ordering()",
	} {
		if !strings.Contains(down, fragment) {
			t.Fatalf("0013 down migration lacks %q", fragment)
		}
	}
}
