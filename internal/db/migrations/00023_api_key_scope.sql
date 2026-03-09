-- +goose Up
ALTER TABLE api_keys ADD COLUMN scope TEXT NOT NULL DEFAULT 'full_access';

-- +goose Down
ALTER TABLE api_keys DROP COLUMN scope;
