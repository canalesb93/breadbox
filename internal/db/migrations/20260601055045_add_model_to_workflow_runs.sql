-- +goose Up
-- Snapshot the model each run was executed with. Lives on the run (not just
-- the workflow definition) so historical runs disclose the model they
-- actually used even after the definition's model is later changed, and so
-- orphaned runs (definition deleted → agent_definition_id NULL) keep the
-- fact. Pre-existing rows default to '' (unknown); the orchestrator stamps
-- every new run at creation.
ALTER TABLE workflow_runs ADD COLUMN model TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE workflow_runs DROP COLUMN model;
