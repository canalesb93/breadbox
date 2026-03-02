-- name: UpsertAccount :one
INSERT INTO accounts (connection_id, external_account_id, name, official_name, type, subtype, mask, iso_currency_code, balance_current, balance_available, balance_limit)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (external_account_id) DO UPDATE SET
  name = EXCLUDED.name,
  official_name = EXCLUDED.official_name,
  type = EXCLUDED.type,
  subtype = EXCLUDED.subtype,
  mask = EXCLUDED.mask,
  balance_current = EXCLUDED.balance_current,
  balance_available = EXCLUDED.balance_available,
  balance_limit = EXCLUDED.balance_limit,
  updated_at = NOW()
RETURNING *;

-- name: ListAccountsByConnection :many
SELECT * FROM accounts WHERE connection_id = $1 ORDER BY name;

-- name: CountAccounts :one
SELECT COUNT(*) FROM accounts;

-- name: UpdateAccountBalances :exec
UPDATE accounts
SET balance_current = $2, balance_available = $3, balance_limit = $4, iso_currency_code = $5, last_balance_update = NOW(), updated_at = NOW()
WHERE external_account_id = $1;

-- name: GetAccountIDByExternalAccountID :one
SELECT id FROM accounts WHERE external_account_id = $1;
