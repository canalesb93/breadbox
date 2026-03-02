-- +goose Up
ALTER TABLE bank_connections ADD COLUMN external_id TEXT;
ALTER TABLE bank_connections ADD COLUMN encrypted_credentials BYTEA;
UPDATE bank_connections SET external_id = plaid_item_id, encrypted_credentials = plaid_access_token;
ALTER TABLE bank_connections DROP COLUMN plaid_item_id;
ALTER TABLE bank_connections DROP COLUMN plaid_access_token;
DROP INDEX IF EXISTS bank_connections_plaid_item_id_idx;
CREATE UNIQUE INDEX bank_connections_provider_external_id_idx ON bank_connections(provider, external_id);

-- +goose Down
ALTER TABLE bank_connections ADD COLUMN plaid_item_id TEXT;
ALTER TABLE bank_connections ADD COLUMN plaid_access_token BYTEA;
UPDATE bank_connections SET plaid_item_id = external_id, plaid_access_token = encrypted_credentials;
ALTER TABLE bank_connections DROP COLUMN external_id;
ALTER TABLE bank_connections DROP COLUMN encrypted_credentials;
DROP INDEX IF EXISTS bank_connections_provider_external_id_idx;
CREATE UNIQUE INDEX bank_connections_plaid_item_id_idx ON bank_connections(plaid_item_id);
