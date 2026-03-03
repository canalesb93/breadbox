-- +goose Up
ALTER TABLE bank_connections ADD COLUMN paused BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE bank_connections ADD COLUMN sync_interval_override_minutes INTEGER NULL;

-- +goose Down
ALTER TABLE bank_connections DROP COLUMN IF EXISTS sync_interval_override_minutes;
ALTER TABLE bank_connections DROP COLUMN IF EXISTS paused;
