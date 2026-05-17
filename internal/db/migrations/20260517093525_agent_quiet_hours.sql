-- +goose Up
-- Per-agent quiet hours — when set, the scheduler silently skips cron
-- fires that land inside the window. Both columns are "HH:MM" 24-hour
-- strings (TEXT not TIME) for simpler JSON wire shape; the scheduler
-- parses + compares against local clock. NULL on either side disables
-- the window.
ALTER TABLE agent_definitions
    ADD COLUMN quiet_hours_start TEXT NULL,
    ADD COLUMN quiet_hours_end   TEXT NULL;

-- +goose Down
ALTER TABLE agent_definitions
    DROP COLUMN quiet_hours_start,
    DROP COLUMN quiet_hours_end;
