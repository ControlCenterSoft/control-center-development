package migrations

import (
	"strings"
	"testing"
)

func TestAuditChainLinearityMigrationContract(t *testing.T) {
	up := readMigration(t, "0011_audit_chain_linearity.up.sql")
	for _, fragment := range []string{
		"ADD CONSTRAINT cc_audit_events_hash_format",
		"CHECK (hash ~ '^[0-9a-f]{64}$')",
		"ADD CONSTRAINT cc_audit_events_previous_hash_format",
		"CHECK (previous_hash IS NULL OR previous_hash ~ '^[0-9a-f]{64}$')",
		"ADD CONSTRAINT cc_audit_events_previous_hash_fk",
		"FOREIGN KEY (previous_hash) REFERENCES cc_audit_events(hash) ON DELETE RESTRICT",
		"CREATE UNIQUE INDEX cc_audit_events_previous_hash_uq",
		"ON cc_audit_events (previous_hash)",
		"WHERE previous_hash IS NOT NULL",
		"CREATE UNIQUE INDEX cc_audit_events_single_genesis_uq",
		"ON cc_audit_events ((1))",
		"WHERE previous_hash IS NULL",
	} {
		if !strings.Contains(up, fragment) {
			t.Fatalf("0011 up migration lacks %q", fragment)
		}
	}

	upper := strings.ToUpper(up)
	for _, forbidden := range []string{"UPDATE CC_AUDIT_EVENTS", "DELETE FROM CC_AUDIT_EVENTS", "TRUNCATE CC_AUDIT_EVENTS", "DROP TABLE CC_AUDIT_EVENTS"} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("0011 migration mutates existing audit rows: found %q", forbidden)
		}
	}

	down := readMigration(t, "0011_audit_chain_linearity.down.sql")
	for _, fragment := range []string{
		"DROP CONSTRAINT IF EXISTS cc_audit_events_previous_hash_fk",
		"DROP CONSTRAINT IF EXISTS cc_audit_events_previous_hash_format",
		"DROP CONSTRAINT IF EXISTS cc_audit_events_hash_format",
		"DROP INDEX IF EXISTS cc_audit_events_single_genesis_uq",
		"DROP INDEX IF EXISTS cc_audit_events_previous_hash_uq",
	} {
		if !strings.Contains(down, fragment) {
			t.Fatalf("0011 down migration lacks %q", fragment)
		}
	}
}
