-- +goose Up
-- transactions.flagged_at: a lightweight "look at this" marker. Set by an agent
-- (or a user) to surface a transaction for human attention. The REASON for a
-- flag is recorded as a comment annotation, not a column — annotations are the
-- canonical historical-context log. NULL = not flagged.
ALTER TABLE transactions ADD COLUMN flagged_at TIMESTAMPTZ NULL;

-- Partial index so the "Flagged" filter (flagged_at IS NOT NULL) stays cheap as
-- the ledger grows — only flagged rows are indexed.
CREATE INDEX transactions_flagged_at_idx ON transactions (flagged_at) WHERE flagged_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS transactions_flagged_at_idx;
ALTER TABLE transactions DROP COLUMN IF EXISTS flagged_at;
