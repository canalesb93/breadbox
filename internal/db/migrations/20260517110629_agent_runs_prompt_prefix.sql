-- +goose Up
-- Operator-supplied prompt prefix for "run now" — the orchestrator prepends
-- this string to the agent's stored prompt for a single manual fire, letting
-- the operator scope the run ("focus on Amazon Prime transactions only")
-- without editing the agent definition. Free-form TEXT; the API/UI caps
-- length; cron runs leave it NULL.
ALTER TABLE agent_runs
    ADD COLUMN prompt_prefix TEXT NULL;

-- +goose Down
ALTER TABLE agent_runs
    DROP COLUMN prompt_prefix;
