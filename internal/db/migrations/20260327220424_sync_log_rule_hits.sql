-- +goose Up
ALTER TABLE sync_logs ADD COLUMN rule_hits JSONB NULL;
COMMENT ON COLUMN sync_logs.rule_hits IS 'Map of transaction rule UUID -> hit count for this sync run';

-- +goose Down
ALTER TABLE sync_logs DROP COLUMN IF EXISTS rule_hits;
