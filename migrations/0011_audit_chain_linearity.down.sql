BEGIN;

ALTER TABLE cc_audit_events
    DROP CONSTRAINT IF EXISTS cc_audit_events_previous_hash_fk,
    DROP CONSTRAINT IF EXISTS cc_audit_events_previous_hash_format,
    DROP CONSTRAINT IF EXISTS cc_audit_events_hash_format;

DROP INDEX IF EXISTS cc_audit_events_single_genesis_uq;
DROP INDEX IF EXISTS cc_audit_events_previous_hash_uq;

COMMIT;
