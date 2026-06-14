-- +goose Up
-- Finish the agent_definitions->workflows / agent_runs->workflow_runs rename
-- (migration 20260531120000) by renaming the FK columns that were "intentionally
-- left as-is to bound the change". After this the subsystem speaks one vocabulary.
-- Index + constraint names are renamed alongside their columns for consistency.

-- workflow_runs.agent_definition_id -> workflow_id
ALTER TABLE workflow_runs RENAME COLUMN agent_definition_id TO workflow_id;
ALTER INDEX agent_runs_definition_started_at_idx RENAME TO workflow_runs_workflow_started_at_idx;
ALTER TABLE workflow_runs RENAME CONSTRAINT agent_runs_agent_definition_id_fkey TO workflow_runs_workflow_id_fkey;

-- api_keys.agent_definition_id -> workflow_id  (durable agent-identity link; see .claude/rules/agents.md)
ALTER TABLE api_keys RENAME COLUMN agent_definition_id TO workflow_id;
ALTER INDEX idx_api_keys_agent_definition_id RENAME TO idx_api_keys_workflow_id;
ALTER TABLE api_keys RENAME CONSTRAINT api_keys_agent_definition_id_fkey TO api_keys_workflow_id_fkey;

-- agent_reports.agent_run_id -> workflow_run_id
ALTER TABLE agent_reports RENAME COLUMN agent_run_id TO workflow_run_id;
ALTER INDEX idx_agent_reports_agent_run_id RENAME TO idx_agent_reports_workflow_run_id;
ALTER TABLE agent_reports RENAME CONSTRAINT agent_reports_agent_run_id_fkey TO agent_reports_workflow_run_id_fkey;

-- +goose Down
ALTER TABLE agent_reports RENAME CONSTRAINT agent_reports_workflow_run_id_fkey TO agent_reports_agent_run_id_fkey;
ALTER INDEX idx_agent_reports_workflow_run_id RENAME TO idx_agent_reports_agent_run_id;
ALTER TABLE agent_reports RENAME COLUMN workflow_run_id TO agent_run_id;

ALTER TABLE api_keys RENAME CONSTRAINT api_keys_workflow_id_fkey TO api_keys_agent_definition_id_fkey;
ALTER INDEX idx_api_keys_workflow_id RENAME TO idx_api_keys_agent_definition_id;
ALTER TABLE api_keys RENAME COLUMN workflow_id TO agent_definition_id;

ALTER TABLE workflow_runs RENAME CONSTRAINT workflow_runs_workflow_id_fkey TO agent_runs_agent_definition_id_fkey;
ALTER INDEX workflow_runs_workflow_started_at_idx RENAME TO agent_runs_definition_started_at_idx;
ALTER TABLE workflow_runs RENAME COLUMN workflow_id TO agent_definition_id;
