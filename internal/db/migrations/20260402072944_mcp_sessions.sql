-- +goose Up

CREATE TABLE mcp_sessions (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id        TEXT        NOT NULL DEFAULT '',
    api_key_id      TEXT        NOT NULL DEFAULT '',
    api_key_name    TEXT        NOT NULL DEFAULT '',
    purpose         TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_mcp_sessions_short_id
    BEFORE INSERT ON mcp_sessions
    FOR EACH ROW EXECUTE FUNCTION set_short_id();

CREATE TABLE mcp_tool_calls (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID        REFERENCES mcp_sessions(id) ON DELETE SET NULL,
    tool_name       TEXT        NOT NULL,
    classification  TEXT        NOT NULL,
    reason          TEXT        NOT NULL DEFAULT '',
    request_json    JSONB,
    response_json   JSONB,
    is_error        BOOLEAN     NOT NULL DEFAULT FALSE,
    actor_type      TEXT        NOT NULL DEFAULT '',
    actor_id        TEXT        NOT NULL DEFAULT '',
    actor_name      TEXT        NOT NULL DEFAULT '',
    duration_ms     INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_mcp_tool_calls_session ON mcp_tool_calls(session_id) WHERE session_id IS NOT NULL;
CREATE INDEX idx_mcp_tool_calls_created ON mcp_tool_calls(created_at DESC);

-- Link reports to sessions
ALTER TABLE agent_reports ADD COLUMN session_id UUID REFERENCES mcp_sessions(id) ON DELETE SET NULL;
CREATE INDEX idx_agent_reports_session ON agent_reports(session_id) WHERE session_id IS NOT NULL;

-- +goose Down
ALTER TABLE agent_reports DROP COLUMN IF EXISTS session_id;
DROP TABLE IF EXISTS mcp_tool_calls;
DROP TABLE IF EXISTS mcp_sessions;
