BEGIN;

DROP TRIGGER IF EXISTS cc_audit_events_no_truncate ON cc_audit_events;

COMMIT;
