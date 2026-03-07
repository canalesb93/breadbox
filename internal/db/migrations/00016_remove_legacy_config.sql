-- +goose Up
DELETE FROM app_config WHERE key IN ('setup_complete', 'admin_username', 'sync_interval_hours');

-- +goose Down
-- No rollback needed — these keys are no longer referenced by the application.
