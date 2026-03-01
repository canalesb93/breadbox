-- +goose Up
CREATE TABLE transactions (
    id                       UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id               UUID          NULL REFERENCES accounts (id) ON DELETE SET NULL,
    external_transaction_id  TEXT          NOT NULL UNIQUE,
    pending_transaction_id   TEXT          NULL,
    amount                   NUMERIC(12,2) NOT NULL,
    iso_currency_code        TEXT          NULL,
    unofficial_currency_code TEXT          NULL,
    date                     DATE          NOT NULL,
    authorized_date          DATE          NULL,
    datetime                 TIMESTAMPTZ   NULL,
    authorized_datetime      TIMESTAMPTZ   NULL,
    name                     TEXT          NOT NULL,
    merchant_name            TEXT          NULL,
    category_primary         TEXT          NULL,
    category_detailed        TEXT          NULL,
    category_confidence      TEXT          NULL,
    payment_channel          TEXT          NULL,
    pending                  BOOLEAN       NOT NULL DEFAULT FALSE,
    deleted_at               TIMESTAMPTZ   NULL,
    created_at               TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX transactions_external_transaction_id_idx ON transactions (external_transaction_id);
CREATE INDEX transactions_account_id_date_idx ON transactions (account_id, date DESC);
CREATE INDEX transactions_account_id_date_active_idx ON transactions (account_id, date DESC) WHERE deleted_at IS NULL;
CREATE INDEX transactions_date_idx ON transactions (date DESC);
CREATE INDEX transactions_pending_idx ON transactions (pending);
CREATE INDEX transactions_category_primary_idx ON transactions (category_primary);
CREATE INDEX transactions_name_merchant_gin_idx ON transactions USING gin (name gin_trgm_ops, merchant_name gin_trgm_ops);
CREATE INDEX transactions_account_id_idx ON transactions (account_id);

-- +goose Down
DROP INDEX IF EXISTS transactions_account_id_idx;
DROP INDEX IF EXISTS transactions_name_merchant_gin_idx;
DROP INDEX IF EXISTS transactions_category_primary_idx;
DROP INDEX IF EXISTS transactions_pending_idx;
DROP INDEX IF EXISTS transactions_date_idx;
DROP INDEX IF EXISTS transactions_account_id_date_active_idx;
DROP INDEX IF EXISTS transactions_account_id_date_idx;
DROP INDEX IF EXISTS transactions_external_transaction_id_idx;
DROP TABLE IF EXISTS transactions;
