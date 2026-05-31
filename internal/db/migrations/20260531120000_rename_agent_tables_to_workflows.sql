-- +goose Up
-- Rename the agent_* tables to workflow_* to match the user-facing
-- "Workflows" product (the UI rename landed in PR4b). Pre-release, so a
-- breaking RENAME is acceptable. PostgreSQL auto-updates the dependent
-- FK constraints (agent_runs.agent_definition_id, agent_reports.agent_run_id,
-- api_keys.agent_definition_id), indexes, and short_id triggers to follow
-- the renamed tables — only the table identifiers change here. FK COLUMN
-- names are intentionally left as-is to bound the change.
ALTER TABLE agent_definitions RENAME TO workflows;
ALTER TABLE agent_runs RENAME TO workflow_runs;

-- +goose Down
ALTER TABLE workflow_runs RENAME TO agent_runs;
ALTER TABLE workflows RENAME TO agent_definitions;
