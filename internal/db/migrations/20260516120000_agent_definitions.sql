-- +goose Up
-- Agent definitions: named, scheduled Claude agents that run locally via
-- the Claude Agent SDK and call breadbox MCP to perform real work.
-- Backs the v2 SPA /agents UI; replaces the v1 admin agent-prompts wizard.
CREATE TABLE agent_definitions (
    id               UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id         TEXT          NOT NULL DEFAULT '',
    name             TEXT          NOT NULL,
    slug             TEXT          NOT NULL UNIQUE,
    prompt           TEXT          NOT NULL,
    system_prompt    TEXT          NULL,
    schedule_cron    TEXT          NULL,
    tool_scope       TEXT          NOT NULL DEFAULT 'read_write'
                         CHECK (tool_scope IN ('read_only', 'read_write')),
    allowed_tools    JSONB         NOT NULL DEFAULT '[]',
    model            TEXT          NOT NULL DEFAULT 'claude-opus-4-7',
    max_turns        INTEGER       NOT NULL DEFAULT 10,
    max_budget_usd   NUMERIC(10,4) NOT NULL DEFAULT 1.0000,
    enabled          BOOLEAN       NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_agent_definitions_short_id
    BEFORE INSERT ON agent_definitions
    FOR EACH ROW EXECUTE FUNCTION set_short_id();

CREATE UNIQUE INDEX agent_definitions_short_id_idx ON agent_definitions (short_id);
CREATE INDEX agent_definitions_enabled_idx ON agent_definitions (enabled) WHERE enabled = TRUE;
CREATE INDEX agent_definitions_created_at_idx ON agent_definitions (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS agent_definitions;
