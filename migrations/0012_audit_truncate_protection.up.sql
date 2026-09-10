BEGIN;

CREATE TRIGGER cc_audit_events_no_truncate
    BEFORE TRUNCATE ON cc_audit_events
    FOR EACH STATEMENT EXECUTE FUNCTION cc_deny_audit_mutation();

COMMIT;
