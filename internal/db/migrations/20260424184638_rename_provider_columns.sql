-- +goose Up
-- Rename raw-provider columns on transactions to use an explicit provider_* prefix,
-- and add provider_raw for storing the unmodified provider payload per transaction.
-- This is a pre-1.0 rename ahead of public release; the intent is that every column
-- that may be enriched later carries a provider_ prefix to make "raw vs enriched"
-- unambiguous at the schema level.

DROP INDEX IF EXISTS transactions_category_primary_idx;
DROP INDEX IF EXISTS transactions_name_merchant_gin_idx;
DROP INDEX IF EXISTS transactions_external_transaction_id_idx;

ALTER TABLE transactions RENAME COLUMN external_transaction_id  TO provider_transaction_id;
ALTER TABLE transactions RENAME COLUMN pending_transaction_id   TO provider_pending_transaction_id;
ALTER TABLE transactions RENAME COLUMN name                     TO provider_name;
ALTER TABLE transactions RENAME COLUMN merchant_name            TO provider_merchant_name;
ALTER TABLE transactions RENAME COLUMN category_primary         TO provider_category_primary;
ALTER TABLE transactions RENAME COLUMN category_detailed        TO provider_category_detailed;
ALTER TABLE transactions RENAME COLUMN category_confidence      TO provider_category_confidence;
ALTER TABLE transactions RENAME COLUMN payment_channel          TO provider_payment_channel;

ALTER TABLE transactions ADD COLUMN provider_raw JSONB NULL;

CREATE INDEX transactions_provider_transaction_id_idx ON transactions (provider_transaction_id);
CREATE INDEX transactions_provider_category_primary_idx ON transactions (provider_category_primary);
CREATE INDEX transactions_provider_name_merchant_gin_idx
  ON transactions USING gin (provider_name gin_trgm_ops, provider_merchant_name gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS transactions_provider_name_merchant_gin_idx;
DROP INDEX IF EXISTS transactions_provider_category_primary_idx;
DROP INDEX IF EXISTS transactions_provider_transaction_id_idx;

ALTER TABLE transactions DROP COLUMN IF EXISTS provider_raw;

ALTER TABLE transactions RENAME COLUMN provider_payment_channel         TO payment_channel;
ALTER TABLE transactions RENAME COLUMN provider_category_confidence     TO category_confidence;
ALTER TABLE transactions RENAME COLUMN provider_category_detailed       TO category_detailed;
ALTER TABLE transactions RENAME COLUMN provider_category_primary        TO category_primary;
ALTER TABLE transactions RENAME COLUMN provider_merchant_name           TO merchant_name;
ALTER TABLE transactions RENAME COLUMN provider_name                    TO name;
ALTER TABLE transactions RENAME COLUMN provider_pending_transaction_id  TO pending_transaction_id;
ALTER TABLE transactions RENAME COLUMN provider_transaction_id          TO external_transaction_id;

CREATE INDEX transactions_external_transaction_id_idx ON transactions (external_transaction_id);
CREATE INDEX transactions_category_primary_idx ON transactions (category_primary);
CREATE INDEX transactions_name_merchant_gin_idx
  ON transactions USING gin (name gin_trgm_ops, merchant_name gin_trgm_ops);
