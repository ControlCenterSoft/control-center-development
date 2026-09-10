package migrations

import (
	"strings"
	"testing"
)

func TestAuditTruncateProtectionMigrationContract(t *testing.T) {
	up := readMigration(t, "0012_audit_truncate_protection.up.sql")
	for _, fragment := range []string{
		"CREATE TRIGGER cc_audit_events_no_truncate",
		"BEFORE TRUNCATE ON cc_audit_events",
		"FOR EACH STATEMENT EXECUTE FUNCTION cc_deny_audit_mutation()",
	} {
		if !strings.Contains(up, fragment) {
			t.Fatalf("0012 up migration lacks %q", fragment)
		}
	}

	upper := strings.ToUpper(up)
	for _, forbidden := range []string{
		"DROP TABLE CC_AUDIT_EVENTS",
		"DELETE FROM CC_AUDIT_EVENTS",
		"UPDATE CC_AUDIT_EVENTS",
		"CREATE OR REPLACE FUNCTION CC_DENY_AUDIT_MUTATION",
	} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("0012 up migration must only add truncate protection: found %q", forbidden)
		}
	}

	down := readMigration(t, "0012_audit_truncate_protection.down.sql")
	if !strings.Contains(down, "DROP TRIGGER IF EXISTS cc_audit_events_no_truncate ON cc_audit_events") {
		t.Fatal("0012 down migration must remove only the truncate trigger")
	}
	if strings.Contains(strings.ToUpper(down), "DROP FUNCTION") {
		t.Fatal("0012 down migration must preserve cc_deny_audit_mutation for UPDATE/DELETE guards")
	}
}
