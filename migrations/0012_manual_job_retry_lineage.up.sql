CREATE TABLE IF NOT EXISTS cc_job_manual_retry_lineage (
    reviewed_admission_id      text        PRIMARY KEY,
    request_fingerprint        text        NOT NULL,
    revalidation_admission_id  text        NOT NULL,
    root_job_id                text        NOT NULL REFERENCES cc_jobs(id),
    source_job_id              text        NOT NULL REFERENCES cc_jobs(id),
    source_job_version         bigint      NOT NULL CHECK (source_job_version > 0),
    retry_job_id               text        NOT NULL UNIQUE REFERENCES cc_jobs(id),
    retry_idempotency_key      text        NOT NULL UNIQUE,
    revision_id                text        NOT NULL,
    revision_digest            text        NOT NULL,
    policy_id                  text        NOT NULL,
    policy_digest              text        NOT NULL,
    retry_history_digest       text        NOT NULL,
    approval_evidence_digest   text,
    requested_at               timestamptz NOT NULL,
    CHECK (root_job_id <> retry_job_id),
    CHECK (source_job_id <> retry_job_id),
    CHECK (char_length(request_fingerprint) = 64),
    CHECK (revision_digest LIKE 'sha256:%'),
    CHECK (policy_digest LIKE 'sha256:%'),
    CHECK (retry_history_digest LIKE 'sha256:%'),
    CHECK (reviewed_admission_id LIKE 'sha256:%'),
    CHECK (revalidation_admission_id LIKE 'sha256:%'),
    CHECK (approval_evidence_digest IS NULL OR approval_evidence_digest LIKE 'sha256:%')
);

CREATE INDEX IF NOT EXISTS cc_job_manual_retry_root_idx
    ON cc_job_manual_retry_lineage (root_job_id, requested_at, retry_job_id);

CREATE INDEX IF NOT EXISTS cc_job_manual_retry_source_idx
    ON cc_job_manual_retry_lineage (source_job_id, source_job_version);
