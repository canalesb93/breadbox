-- name: CountTransactions :one
SELECT COUNT(*) FROM transactions WHERE deleted_at IS NULL;

-- name: UpsertTransaction :one
INSERT INTO transactions (
  account_id, external_transaction_id, pending_transaction_id,
  amount, iso_currency_code, date, authorized_date,
  datetime, authorized_datetime, name, merchant_name,
  category_primary, category_detailed, category_confidence,
  payment_channel, pending
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
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
  deleted_at = NULL,
  updated_at = NOW()
RETURNING *;

-- name: SoftDeleteTransactionByExternalID :exec
UPDATE transactions SET deleted_at = NOW() WHERE external_transaction_id = $1 AND deleted_at IS NULL;

-- name: GetTransaction :one
SELECT * FROM transactions WHERE id = $1 AND deleted_at IS NULL;
