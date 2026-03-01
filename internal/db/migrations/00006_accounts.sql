-- +goose Up
CREATE TABLE accounts (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    connection_id       UUID          NULL REFERENCES bank_connections (id) ON DELETE SET NULL,
    external_account_id TEXT          NOT NULL UNIQUE,
    name                TEXT          NOT NULL,
    official_name       TEXT          NULL,
    type                TEXT          NOT NULL,
    subtype             TEXT          NULL,
    mask                TEXT          NULL,
    balance_current     NUMERIC(12,2) NULL,
    balance_available   NUMERIC(12,2) NULL,
    balance_limit       NUMERIC(12,2) NULL,
    iso_currency_code   TEXT          NULL,
    last_balance_update TIMESTAMPTZ   NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX accounts_connection_id_idx ON accounts (connection_id);
CREATE INDEX accounts_external_account_id_idx ON accounts (external_account_id);

-- +goose Down
DROP INDEX IF EXISTS accounts_external_account_id_idx;
DROP INDEX IF EXISTS accounts_connection_id_idx;
DROP TABLE IF EXISTS accounts;
