-- +goose Up
ALTER TABLE accounts ADD COLUMN display_name TEXT NULL;
ALTER TABLE accounts ADD COLUMN excluded BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE accounts DROP COLUMN IF EXISTS excluded;
ALTER TABLE accounts DROP COLUMN IF EXISTS display_name;
