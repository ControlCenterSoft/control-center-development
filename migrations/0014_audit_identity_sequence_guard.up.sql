BEGIN;

DO $block$
DECLARE
    identity_sequence regclass;
    sequence_schema name;
    sequence_name name;
    sequence_last bigint;
    sequence_increment bigint;
    sequence_cycle boolean;
    sequence_cache bigint;
    maximum_event_sequence bigint;
BEGIN
    identity_sequence := pg_get_serial_sequence('cc_audit_events', 'sequence_id')::regclass;
    IF identity_sequence IS NULL THEN
        RAISE EXCEPTION 'cc_audit_events sequence_id identity sequence is unavailable';
    END IF;

    SELECT namespace.nspname, relation.relname
      INTO sequence_schema, sequence_name
      FROM pg_class AS relation
      JOIN pg_namespace AS namespace ON namespace.oid = relation.relnamespace
     WHERE relation.oid = identity_sequence;

    SELECT last_value, increment_by, cycle, cache_size
      INTO sequence_last, sequence_increment, sequence_cycle, sequence_cache
      FROM pg_sequences
     WHERE schemaname = sequence_schema
       AND sequencename = sequence_name;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'cc_audit_events identity sequence metadata is unavailable';
    END IF;

    IF sequence_increment <> 1 OR sequence_cycle OR sequence_cache <> 1 THEN
        RAISE EXCEPTION 'cc_audit_events identity sequence must use increment 1, NO CYCLE, CACHE 1';
    END IF;

    SELECT max(sequence_id)
      INTO maximum_event_sequence
      FROM cc_audit_events;

    IF maximum_event_sequence IS NOT NULL
       AND (sequence_last IS NULL OR maximum_event_sequence > sequence_last) THEN
        RAISE EXCEPTION 'cc_audit_events sequence_id is ahead of its identity sequence';
    END IF;
END
$block$;

CREATE OR REPLACE FUNCTION cc_enforce_audit_identity_sequence()
RETURNS trigger
LANGUAGE plpgsql
AS $function$
DECLARE
    identity_sequence regclass;
    sequence_schema name;
    sequence_name name;
    sequence_last bigint;
    sequence_increment bigint;
    sequence_cycle boolean;
    sequence_cache bigint;
BEGIN
    identity_sequence := pg_get_serial_sequence('cc_audit_events', 'sequence_id')::regclass;
    IF identity_sequence IS NULL THEN
        RAISE EXCEPTION 'cc_audit_events sequence_id identity sequence is unavailable';
    END IF;

    SELECT namespace.nspname, relation.relname
      INTO sequence_schema, sequence_name
      FROM pg_class AS relation
      JOIN pg_namespace AS namespace ON namespace.oid = relation.relnamespace
     WHERE relation.oid = identity_sequence;

    SELECT last_value, increment_by, cycle, cache_size
      INTO sequence_last, sequence_increment, sequence_cycle, sequence_cache
      FROM pg_sequences
     WHERE schemaname = sequence_schema
       AND sequencename = sequence_name;

    IF NOT FOUND
       OR sequence_last IS NULL
       OR sequence_increment <> 1
       OR sequence_cycle
       OR sequence_cache <> 1 THEN
        RAISE EXCEPTION 'cc_audit_events identity sequence state is not canonical';
    END IF;

    IF NEW.sequence_id > sequence_last THEN
        RAISE EXCEPTION 'cc_audit_events sequence_id must not advance beyond identity sequence state';
    END IF;

    RETURN NEW;
END;
$function$;

CREATE TRIGGER cc_audit_events_identity_sequence_guard
    BEFORE INSERT ON cc_audit_events
    FOR EACH ROW EXECUTE FUNCTION cc_enforce_audit_identity_sequence();

COMMIT;
