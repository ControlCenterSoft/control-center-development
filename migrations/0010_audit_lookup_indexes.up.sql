BEGIN;

CREATE INDEX IF NOT EXISTS cc_audit_events_correlation_sequence_idx
    ON cc_audit_events (correlation_id, sequence_id DESC)
    WHERE correlation_id IS NOT NULL;

COMMIT;
