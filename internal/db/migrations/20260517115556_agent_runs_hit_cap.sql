-- +goose Up
-- hit_cap records WHICH safety ceiling a run bumped into when it terminated.
-- Values:
--   NULL          — run finished normally (clean success, or a non-cap error)
--   'max_turns'   — sidecar reported stop_reason=max_turns (clean term within bounds)
--   'max_budget'  — sidecar reported stop_reason=budget_exceeded (mid-run abort)
-- Lets the v2 SPA flag "ran into the ceiling" runs separately from clean
-- successes; an operator who sees a max_turns row knows the work is
-- probably incomplete and may want to raise max_turns or split the prompt.
ALTER TABLE agent_runs
    ADD COLUMN hit_cap TEXT NULL CHECK (hit_cap IN ('max_turns', 'max_budget'));

-- +goose Down
ALTER TABLE agent_runs
    DROP COLUMN hit_cap;
