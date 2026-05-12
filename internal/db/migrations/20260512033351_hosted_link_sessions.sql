-- +goose Up

-- hosted_link_sessions backs the agent-shareable bank-linking surface.
-- An agent mints a session + token; the user opens the link in a browser
-- and the page (added in a later PR) wraps Plaid Link / Teller Connect.
-- The plaintext token is never stored; only its SHA-256 hash sits in
-- token_hash and the bearer middleware (PR3) compares hashes there.
--
-- Status lifecycle: pending -> active (page opened) -> completed | failed.
-- expires_at is enforced in the service layer and (later) by a cleanup job.
-- This migration is additive — safe for the shared dev DB.

CREATE TABLE hosted_link_sessions (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id              TEXT NOT NULL DEFAULT '',
    token_hash            TEXT NOT NULL UNIQUE,
    user_id               UUID NOT NULL REFERENCES users(id),
    provider              TEXT,
    action                TEXT NOT NULL,
    connection_id         UUID REFERENCES bank_connections(id) ON DELETE SET NULL,
    single_use            BOOLEAN NOT NULL DEFAULT FALSE,
    redirect_url          TEXT,
    label                 TEXT,
    status                TEXT NOT NULL DEFAULT 'pending',
    error_code            TEXT,
    error_message         TEXT,
    result_connection_ids UUID[] NOT NULL DEFAULT '{}',
    expires_at            TIMESTAMPTZ NOT NULL,
    started_at            TIMESTAMPTZ,
    completed_at          TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX hosted_link_sessions_short_id_idx ON hosted_link_sessions (short_id);
CREATE INDEX hosted_link_sessions_user_id_idx ON hosted_link_sessions (user_id);

CREATE TRIGGER set_hosted_link_sessions_short_id
    BEFORE INSERT ON hosted_link_sessions
    FOR EACH ROW EXECUTE FUNCTION set_short_id();

-- +goose Down
DROP TRIGGER IF EXISTS set_hosted_link_sessions_short_id ON hosted_link_sessions;
DROP TABLE IF EXISTS hosted_link_sessions;
