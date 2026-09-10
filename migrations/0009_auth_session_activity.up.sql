ALTER TABLE cc_auth_sessions
    ADD COLUMN last_activity_at timestamptz;

UPDATE cc_auth_sessions
SET last_activity_at = created_at
WHERE last_activity_at IS NULL;

ALTER TABLE cc_auth_sessions
    ALTER COLUMN last_activity_at SET NOT NULL,
    ADD CONSTRAINT cc_auth_sessions_activity_window
        CHECK (last_activity_at >= created_at AND last_activity_at <= expires_at);

CREATE INDEX cc_auth_sessions_user_activity_idx
    ON cc_auth_sessions (user_id, last_activity_at DESC)
    WHERE revoked_at IS NULL;
