-- name: UpsertTransactionProviderPayload :exec
-- Stores the raw provider payload for a transaction in the 1:1 satellite table,
-- keeping the heavy JSONB off the hot transactions row. Called right after the
-- transaction upsert, only when a non-empty payload is available.
INSERT INTO transaction_provider_payloads (transaction_id, provider_raw, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (transaction_id) DO UPDATE SET
  provider_raw = EXCLUDED.provider_raw,
  updated_at = now();

-- name: GetTransactionProviderPayload :one
SELECT provider_raw FROM transaction_provider_payloads WHERE transaction_id = $1;
