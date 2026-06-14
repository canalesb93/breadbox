-- +goose Up
-- Drop the dead unofficial_currency_code column. It was a Plaid-era dual-currency
-- field (cryptocurrency / non-standard currency) that has never been written by any
-- provider in Breadbox and is never read. Removing it for the v1 schema cleanup.
ALTER TABLE transactions DROP COLUMN IF EXISTS unofficial_currency_code;

-- +goose Down
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS unofficial_currency_code TEXT NULL;
