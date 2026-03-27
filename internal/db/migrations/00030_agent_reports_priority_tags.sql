-- +goose Up
ALTER TABLE agent_reports ADD COLUMN priority TEXT NOT NULL DEFAULT 'info';
ALTER TABLE agent_reports ADD COLUMN tags TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE agent_reports ADD COLUMN author TEXT NULL;

CREATE INDEX agent_reports_priority_idx ON agent_reports (priority) WHERE read_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS agent_reports_priority_idx;
ALTER TABLE agent_reports DROP COLUMN IF EXISTS author;
ALTER TABLE agent_reports DROP COLUMN IF EXISTS tags;
ALTER TABLE agent_reports DROP COLUMN IF EXISTS priority;
