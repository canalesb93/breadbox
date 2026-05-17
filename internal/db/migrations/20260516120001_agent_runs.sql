-- +goose Up
-- Agent runs: execution history for agent_definitions, mirroring sync_logs
-- in structure. ON DELETE SET NULL preserves history when a definition is deleted.
CREATE TABLE agent_runs (
    id                    UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id              TEXT          NOT NULL DEFAULT '',
    agent_definition_id   UUID          REFERENCES agent_definitions (id) ON DELETE SET NULL,
    "trigger"             TEXT          NOT NULL
                              CHECK ("trigger" IN ('cron', 'manual', 'webhook')),
    status                TEXT          NOT NULL DEFAULT 'in_progress'
                              CHECK (status IN ('in_progress', 'success', 'error', 'timeout', 'skipped')),
    started_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    completed_at          TIMESTAMPTZ   NULL,
    duration_ms           INTEGER       NULL,
    total_cost_usd        NUMERIC(10,4) NULL,
    input_tokens          INTEGER       NULL,
    output_tokens         INTEGER       NULL,
    cache_read_tokens     INTEGER       NULL,
    cache_creation_tokens INTEGER       NULL,
    turn_count            INTEGER       NULL,
    max_turns_used        INTEGER       NULL,
    num_tool_calls        INTEGER       NULL,
    error_message         TEXT          NULL,
    transcript_path       TEXT          NULL,
    session_id            TEXT          NULL
);

CREATE TRIGGER set_agent_runs_short_id
    BEFORE INSERT ON agent_runs
    FOR EACH ROW EXECUTE FUNCTION set_short_id();

CREATE UNIQUE INDEX agent_runs_short_id_idx ON agent_runs (short_id);
CREATE INDEX agent_runs_definition_started_at_idx
    ON agent_runs (agent_definition_id, started_at DESC);
CREATE INDEX agent_runs_started_at_idx ON agent_runs (started_at DESC);
CREATE INDEX agent_runs_status_idx ON agent_runs (status);

-- +goose Down
DROP TABLE IF EXISTS agent_runs;
