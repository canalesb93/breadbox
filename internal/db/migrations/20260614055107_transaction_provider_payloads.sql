-- +goose Up
-- Move transactions.provider_raw (full raw provider payload, write-only / never
-- read) off the hot ledger into a 1:1 satellite table. Keeps row scans on the
-- frequently-queried transactions table lean while preserving every payload for
-- debugging and future re-enrichment (available on join, keyed by transaction_id).
CREATE TABLE transaction_provider_payloads (
    transaction_id uuid PRIMARY KEY REFERENCES transactions(id) ON DELETE CASCADE,
    provider_raw   jsonb       NOT NULL,
    updated_at     timestamptz NOT NULL DEFAULT now()
);

INSERT INTO transaction_provider_payloads (transaction_id, provider_raw)
SELECT id, provider_raw FROM transactions WHERE provider_raw IS NOT NULL;

ALTER TABLE transactions DROP COLUMN provider_raw;

-- +goose Down
ALTER TABLE transactions ADD COLUMN provider_raw jsonb;
UPDATE transactions t
   SET provider_raw = p.provider_raw
  FROM transaction_provider_payloads p
 WHERE p.transaction_id = t.id;
DROP TABLE transaction_provider_payloads;
