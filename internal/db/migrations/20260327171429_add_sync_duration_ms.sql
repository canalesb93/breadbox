-- +goose Up
ALTER TABLE sync_logs ADD COLUMN duration_ms INTEGER NULL;

-- Backfill duration_ms for existing completed sync logs.
UPDATE sync_logs
SET duration_ms = EXTRACT(MILLISECONDS FROM (completed_at - started_at))::INTEGER
WHERE completed_at IS NOT NULL AND started_at IS NOT NULL;

-- +goose Down
ALTER TABLE sync_logs DROP COLUMN IF EXISTS duration_ms;
