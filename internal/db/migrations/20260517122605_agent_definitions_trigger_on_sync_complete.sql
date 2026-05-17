-- +goose Up
-- trigger_on_sync_complete flips an agent into "fire after each successful
-- sync" mode. Useful for keep-up agents like "re-categorize freshly synced
-- transactions" — the operator wires the agent to a sync-finished event
-- instead of (or in addition to) a cron schedule. Defaults to false so
-- existing agents are unaffected; opt-in per definition via the v2 SPA
-- edit form.
ALTER TABLE agent_definitions
    ADD COLUMN trigger_on_sync_complete BOOLEAN NOT NULL DEFAULT FALSE;

-- Lookup index: post-sync hook reads "all enabled agents with this flag".
CREATE INDEX agent_definitions_trigger_on_sync_complete_idx
    ON agent_definitions (trigger_on_sync_complete)
    WHERE trigger_on_sync_complete = TRUE AND enabled = TRUE;

-- +goose Down
DROP INDEX IF EXISTS agent_definitions_trigger_on_sync_complete_idx;
ALTER TABLE agent_definitions
    DROP COLUMN trigger_on_sync_complete;
