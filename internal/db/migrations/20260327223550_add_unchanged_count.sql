-- +goose Up
ALTER TABLE sync_logs ADD COLUMN unchanged_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sync_log_accounts ADD COLUMN unchanged_count INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE sync_log_accounts DROP COLUMN IF EXISTS unchanged_count;
ALTER TABLE sync_logs DROP COLUMN IF EXISTS unchanged_count;
