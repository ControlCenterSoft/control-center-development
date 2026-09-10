BEGIN;

ALTER TABLE cc_audit_events
    ADD CONSTRAINT cc_audit_events_hash_format
    CHECK (hash ~ '^[0-9a-f]{64}$'),
    ADD CONSTRAINT cc_audit_events_previous_hash_format
    CHECK (previous_hash IS NULL OR previous_hash ~ '^[0-9a-f]{64}$'),
    ADD CONSTRAINT cc_audit_events_previous_hash_fk
    FOREIGN KEY (previous_hash) REFERENCES cc_audit_events(hash) ON DELETE RESTRICT;

CREATE UNIQUE INDEX cc_audit_events_previous_hash_uq
    ON cc_audit_events (previous_hash)
    WHERE previous_hash IS NOT NULL;

CREATE UNIQUE INDEX cc_audit_events_single_genesis_uq
    ON cc_audit_events ((1))
    WHERE previous_hash IS NULL;

COMMIT;
