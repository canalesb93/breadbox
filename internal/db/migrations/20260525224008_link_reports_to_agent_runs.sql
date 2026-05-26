-- +goose Up
-- Link agent_reports rows to the agent_runs row that produced them.
--
-- An agent calls submit_report via MCP while it's running; the MCP
-- server can resolve the surrounding run via the per-run minted API
-- key name (`agent:<slug>:<run-short-id>`) and stamp the FK on the
-- new row. The runs landing page reads the reverse — runs joined to
-- reports — so the row can show a "Report: Title" chip inline.
--
-- The column is nullable because not every report comes from a run
-- (operator-submitted reports, MCP sessions launched outside the
-- agent SDK, future report sources). ON DELETE SET NULL preserves
-- the report when its source run is purged by retention.
ALTER TABLE agent_reports
    ADD COLUMN agent_run_id UUID NULL REFERENCES agent_runs(id) ON DELETE SET NULL;

CREATE INDEX idx_agent_reports_agent_run_id
    ON agent_reports (agent_run_id)
    WHERE agent_run_id IS NOT NULL;

-- Backfill: derive agent_run_id from the API key name format
-- `agent:<slug>:<runShortID>` for reports that already have a
-- session_id pointing at an MCP session for an agent run. Best-effort;
-- rows without a matching key/run stay NULL.
UPDATE agent_reports r
SET agent_run_id = ar.id
FROM mcp_sessions ms
JOIN agent_runs ar ON ar.short_id = SPLIT_PART(ms.api_key_name, ':', 3)
WHERE r.session_id = ms.id
  AND r.agent_run_id IS NULL
  AND ms.api_key_name LIKE 'agent:%:%';

-- +goose Down
DROP INDEX IF EXISTS idx_agent_reports_agent_run_id;
ALTER TABLE agent_reports DROP COLUMN IF EXISTS agent_run_id;
