-- +goose Up
ALTER TABLE auth_accounts ADD COLUMN setup_token TEXT UNIQUE;
ALTER TABLE auth_accounts ADD COLUMN setup_token_expires_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE auth_accounts DROP COLUMN IF EXISTS setup_token_expires_at;
ALTER TABLE auth_accounts DROP COLUMN IF EXISTS setup_token;
