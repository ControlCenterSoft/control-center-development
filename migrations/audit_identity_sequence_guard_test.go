package migrations

import (
	"strings"
	"testing"
)

func TestAuditIdentitySequenceGuardMigrationContract(t *testing.T) {
	up := readMigration(t, "0014_audit_identity_sequence_guard.up.sql")
	for _, fragment := range []string{
		"pg_get_serial_sequence('cc_audit_events', 'sequence_id')::regclass",
		"FROM pg_sequences",
		"sequence_increment <> 1 OR sequence_cycle OR sequence_cache <> 1",
		"maximum_event_sequence > sequence_last",
		"CREATE OR REPLACE FUNCTION cc_enforce_audit_identity_sequence()",
		"IF NEW.sequence_id > sequence_last THEN",
		"CREATE TRIGGER cc_audit_events_identity_sequence_guard",
		"BEFORE INSERT ON cc_audit_events",
		"FOR EACH ROW EXECUTE FUNCTION cc_enforce_audit_identity_sequence()",
	} {
		if !strings.Contains(up, fragment) {
			t.Fatalf("0014 up migration lacks %q", fragment)
		}
	}

	upper := strings.ToUpper(up)
	for _, forbidden := range []string{
		"SETVAL(",
		"ALTER SEQUENCE",
		"UPDATE CC_AUDIT_EVENTS",
		"DELETE FROM CC_AUDIT_EVENTS",
		"TRUNCATE CC_AUDIT_EVENTS",
		"DROP TABLE CC_AUDIT_EVENTS",
	} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("0014 up migration must fail closed without rewriting audit or sequence state: found %q", forbidden)
		}
	}

	down := readMigration(t, "0014_audit_identity_sequence_guard.down.sql")
	for _, fragment := range []string{
		"DROP TRIGGER IF EXISTS cc_audit_events_identity_sequence_guard ON cc_audit_events",
		"DROP FUNCTION IF EXISTS cc_enforce_audit_identity_sequence()",
	} {
		if !strings.Contains(down, fragment) {
			t.Fatalf("0014 down migration lacks %q", fragment)
		}
	}
}
