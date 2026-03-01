-- +goose Up
CREATE TABLE bank_connections (
    id                      UUID              PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                 UUID              NULL REFERENCES users (id) ON DELETE SET NULL,
    provider                provider_type     NOT NULL,
    institution_id          TEXT              NULL,
    institution_name        TEXT              NULL,
    plaid_item_id           TEXT              NULL UNIQUE,
    plaid_access_token      BYTEA             NULL,
    sync_cursor             TEXT              NULL,
    status                  connection_status NOT NULL DEFAULT 'active',
    error_code              TEXT              NULL,
    error_message           TEXT              NULL,
    new_accounts_available  BOOLEAN           NOT NULL DEFAULT FALSE,
    consent_expiration_time TIMESTAMPTZ       NULL,
    last_synced_at          TIMESTAMPTZ       NULL,
    created_at              TIMESTAMPTZ       NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ       NOT NULL DEFAULT NOW()
);

CREATE INDEX bank_connections_user_id_idx ON bank_connections (user_id);
CREATE INDEX bank_connections_status_idx ON bank_connections (status);
CREATE INDEX bank_connections_plaid_item_id_idx ON bank_connections (plaid_item_id);

-- +goose Down
DROP INDEX IF EXISTS bank_connections_plaid_item_id_idx;
DROP INDEX IF EXISTS bank_connections_status_idx;
DROP INDEX IF EXISTS bank_connections_user_id_idx;
DROP TABLE IF EXISTS bank_connections;
