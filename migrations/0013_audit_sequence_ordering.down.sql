BEGIN;

DROP TRIGGER IF EXISTS cc_audit_events_sequence_ordering ON cc_audit_events;
DROP FUNCTION IF EXISTS cc_enforce_audit_sequence_ordering();

COMMIT;
