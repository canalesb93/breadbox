-- Add 'cancelled' to the workflow_runs.status set so an operator can stop a run
-- mid-flight (the run-detail "Cancel run" button) and have it land as an
-- intentional terminal status rather than a generic 'error'. The new set is a
-- strict superset of the old, so this is additive: existing rows and concurrent
-- inserts (which only ever write the prior statuses) keep validating. The
-- constraint kept its original auto-generated name (agent_runs_status_check)
-- when the table was renamed agent_runs -> workflow_runs.

-- +goose Up
ALTER TABLE workflow_runs DROP CONSTRAINT agent_runs_status_check;
ALTER TABLE workflow_runs ADD CONSTRAINT agent_runs_status_check
    CHECK (status IN ('in_progress', 'success', 'error', 'timeout', 'skipped', 'cancelled'));

-- +goose Down
ALTER TABLE workflow_runs DROP CONSTRAINT agent_runs_status_check;
ALTER TABLE workflow_runs ADD CONSTRAINT agent_runs_status_check
    CHECK (status IN ('in_progress', 'success', 'error', 'timeout', 'skipped'));
