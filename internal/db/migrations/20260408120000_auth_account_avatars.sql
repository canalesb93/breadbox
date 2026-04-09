-- +goose Up
-- Cleanup: remove auth_account avatar columns added during development.
-- Avatar functionality lives on the users table only.
-- Unlinked admin accounts get a generated pattern from their account ID.
ALTER TABLE auth_accounts DROP COLUMN IF EXISTS avatar_data;
ALTER TABLE auth_accounts DROP COLUMN IF EXISTS avatar_content_type;
ALTER TABLE auth_accounts DROP COLUMN IF EXISTS avatar_seed;

-- +goose Down
ALTER TABLE auth_accounts ADD COLUMN avatar_data BYTEA;
ALTER TABLE auth_accounts ADD COLUMN avatar_content_type TEXT;
ALTER TABLE auth_accounts ADD COLUMN avatar_seed TEXT;
