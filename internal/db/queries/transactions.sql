-- name: CountTransactions :one
SELECT COUNT(*) FROM transactions WHERE deleted_at IS NULL;

-- name: UpsertTransaction :one
INSERT INTO transactions (
  account_id, provider_transaction_id, provider_pending_transaction_id,
  amount, iso_currency_code, date, authorized_date,
  datetime, authorized_datetime, provider_name, provider_merchant_name,
  provider_category_primary, provider_category_detailed, provider_category_confidence,
  provider_payment_channel, pending, category_id, provider_raw
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
)
ON CONFLICT (provider_transaction_id) DO UPDATE SET
  account_id = EXCLUDED.account_id,
  provider_pending_transaction_id = EXCLUDED.provider_pending_transaction_id,
  amount = EXCLUDED.amount,
  iso_currency_code = EXCLUDED.iso_currency_code,
  date = EXCLUDED.date,
  authorized_date = EXCLUDED.authorized_date,
  datetime = EXCLUDED.datetime,
  authorized_datetime = EXCLUDED.authorized_datetime,
  provider_name = EXCLUDED.provider_name,
  provider_merchant_name = EXCLUDED.provider_merchant_name,
  provider_category_primary = EXCLUDED.provider_category_primary,
  provider_category_detailed = EXCLUDED.provider_category_detailed,
  provider_category_confidence = EXCLUDED.provider_category_confidence,
  provider_payment_channel = EXCLUDED.provider_payment_channel,
  pending = EXCLUDED.pending,
  provider_raw = COALESCE(EXCLUDED.provider_raw, transactions.provider_raw),
  category_id = CASE
    WHEN transactions.category_override THEN transactions.category_id
    WHEN EXCLUDED.category_id IS NOT NULL THEN EXCLUDED.category_id
    ELSE transactions.category_id
  END,
  deleted_at = NULL,
  updated_at = CASE
    WHEN transactions.amount IS DISTINCT FROM EXCLUDED.amount
      OR transactions.provider_name IS DISTINCT FROM EXCLUDED.provider_name
      OR transactions.pending IS DISTINCT FROM EXCLUDED.pending
      OR transactions.provider_merchant_name IS DISTINCT FROM EXCLUDED.provider_merchant_name
      OR transactions.provider_category_primary IS DISTINCT FROM EXCLUDED.provider_category_primary
      OR transactions.provider_category_detailed IS DISTINCT FROM EXCLUDED.provider_category_detailed
      OR transactions.deleted_at IS NOT NULL
    THEN NOW()
    ELSE transactions.updated_at
  END
RETURNING *;

-- name: SoftDeleteTransactionByExternalID :exec
UPDATE transactions SET deleted_at = NOW() WHERE provider_transaction_id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteTransactionsByConnectionID :execrows
UPDATE transactions SET deleted_at = NOW()
WHERE account_id IN (SELECT id FROM accounts WHERE connection_id = $1)
  AND deleted_at IS NULL;

-- name: GetTransaction :one
SELECT * FROM transactions WHERE id = $1 AND deleted_at IS NULL;

-- name: GetTransactionUUIDByShortID :one
SELECT id FROM transactions WHERE short_id = $1 AND deleted_at IS NULL;
