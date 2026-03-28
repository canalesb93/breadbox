-- name: CountTransactions :one
SELECT COUNT(*) FROM transactions WHERE deleted_at IS NULL;

-- name: UpsertTransaction :one
INSERT INTO transactions (
  account_id, external_transaction_id, pending_transaction_id,
  amount, iso_currency_code, date, authorized_date,
  datetime, authorized_datetime, name, merchant_name,
  category_primary, category_detailed, category_confidence,
  payment_channel, pending, category_id
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
)
ON CONFLICT (external_transaction_id) DO UPDATE SET
  account_id = EXCLUDED.account_id,
  pending_transaction_id = EXCLUDED.pending_transaction_id,
  amount = EXCLUDED.amount,
  iso_currency_code = EXCLUDED.iso_currency_code,
  date = EXCLUDED.date,
  authorized_date = EXCLUDED.authorized_date,
  datetime = EXCLUDED.datetime,
  authorized_datetime = EXCLUDED.authorized_datetime,
  name = EXCLUDED.name,
  merchant_name = EXCLUDED.merchant_name,
  category_primary = EXCLUDED.category_primary,
  category_detailed = EXCLUDED.category_detailed,
  category_confidence = EXCLUDED.category_confidence,
  payment_channel = EXCLUDED.payment_channel,
  pending = EXCLUDED.pending,
  category_id = CASE WHEN transactions.category_override THEN transactions.category_id ELSE EXCLUDED.category_id END,
  deleted_at = NULL,
  updated_at = CASE
    WHEN transactions.amount IS DISTINCT FROM EXCLUDED.amount
      OR transactions.name IS DISTINCT FROM EXCLUDED.name
      OR transactions.pending IS DISTINCT FROM EXCLUDED.pending
      OR transactions.merchant_name IS DISTINCT FROM EXCLUDED.merchant_name
      OR transactions.category_primary IS DISTINCT FROM EXCLUDED.category_primary
      OR transactions.category_detailed IS DISTINCT FROM EXCLUDED.category_detailed
      OR transactions.deleted_at IS NOT NULL
    THEN NOW()
    ELSE transactions.updated_at
  END
RETURNING *;

-- name: SoftDeleteTransactionByExternalID :exec
UPDATE transactions SET deleted_at = NOW() WHERE external_transaction_id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteTransactionsByConnectionID :execrows
UPDATE transactions SET deleted_at = NOW()
WHERE account_id IN (SELECT id FROM accounts WHERE connection_id = $1)
  AND deleted_at IS NULL;

-- name: GetTransaction :one
SELECT * FROM transactions WHERE id = $1 AND deleted_at IS NULL;
