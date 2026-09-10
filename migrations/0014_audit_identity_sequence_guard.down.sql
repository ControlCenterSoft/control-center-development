BEGIN;

DROP TRIGGER IF EXISTS cc_audit_events_identity_sequence_guard ON cc_audit_events;
DROP FUNCTION IF EXISTS cc_enforce_audit_identity_sequence();

COMMIT;
