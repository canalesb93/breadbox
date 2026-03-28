-- +goose Up
ALTER TABLE sync_logs ADD COLUMN warning_message TEXT NULL;

-- +goose Down
ALTER TABLE sync_logs DROP COLUMN IF EXISTS warning_message;
