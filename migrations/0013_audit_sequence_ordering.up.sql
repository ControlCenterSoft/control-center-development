BEGIN;

DO $block$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM cc_audit_events AS child
        JOIN cc_audit_events AS predecessor
          ON predecessor.hash = child.previous_hash
        WHERE child.sequence_id <= predecessor.sequence_id
    ) THEN
        RAISE EXCEPTION 'cc_audit_events contains non-monotonic sequence/hash linkage';
    END IF;
END
$block$;

CREATE OR REPLACE FUNCTION cc_enforce_audit_sequence_ordering()
RETURNS trigger
LANGUAGE plpgsql
AS $function$
DECLARE
    predecessor_sequence bigint;
BEGIN
    IF NEW.previous_hash IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT sequence_id
      INTO predecessor_sequence
      FROM cc_audit_events
     WHERE hash = NEW.previous_hash;

    IF predecessor_sequence IS NULL THEN
        RAISE EXCEPTION 'cc_audit_events predecessor must exist before sequence validation';
    END IF;

    IF NEW.sequence_id <= predecessor_sequence THEN
        RAISE EXCEPTION 'cc_audit_events sequence_id must advance predecessor sequence_id';
    END IF;

    RETURN NEW;
END;
$function$;

CREATE CONSTRAINT TRIGGER cc_audit_events_sequence_ordering
    AFTER INSERT ON cc_audit_events
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION cc_enforce_audit_sequence_ordering();

COMMIT;
