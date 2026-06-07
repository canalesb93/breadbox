-- +goose Up

-- content_hash: an account-INDEPENDENT fingerprint of a transaction's core
-- content (date | amount | normalized description). The existing dedup key
-- `provider_transaction_id` bakes in the account id, so it only catches exact
-- re-imports into the SAME account. The CSV import v2 classifier needs to detect
-- duplicates against ALL transactions already in a resolved account — including
-- Plaid/Teller rows — so it compares on this account-independent hash plus a
-- date+amount+name fuzzy pass.
--
-- Nullable + additive: CSV v2 writes it going forward; legacy provider rows stay
-- NULL (the classifier falls back to date+amount+name for those). Safe on the
-- shared dev DB alongside running servers.
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS content_hash TEXT;

-- Fast candidate lookup for classification: "all live transactions in this
-- account near this date with this amount". Partial on deleted_at because the
-- classifier only ever compares against live rows.
CREATE INDEX IF NOT EXISTS transactions_dedup_lookup
    ON transactions (account_id, date, amount)
    WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS transactions_dedup_lookup;
ALTER TABLE transactions DROP COLUMN IF EXISTS content_hash;
