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

-- name: UpsertAccountMetadata :one
-- Inserts a freshly discovered account or refreshes the metadata of an existing
-- one WITHOUT touching balances. Used by the sync engine when a provider returns
-- its current account set mid-sync (e.g. SimpleFIN, where one access URL grows
-- new accounts as the user links banks at the bridge). Balances are owned by the
-- separate balance-refresh path, so this upsert deliberately omits them to avoid
-- clobbering live values with NULLs. connection_id is set only on INSERT; an
-- existing account keeps its connection.
INSERT INTO accounts (connection_id, external_account_id, name, official_name, type, subtype, mask, iso_currency_code)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (external_account_id) DO UPDATE SET
  name = EXCLUDED.name,
  official_name = EXCLUDED.official_name,
  type = EXCLUDED.type,
  subtype = EXCLUDED.subtype,
  mask = EXCLUDED.mask,
  iso_currency_code = EXCLUDED.iso_currency_code,
  updated_at = NOW()
RETURNING id, external_account_id, name, display_name;

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

-- name: GetAccountIDAndNameByExternalAccountID :one
SELECT id, COALESCE(display_name, name) AS name
FROM accounts WHERE external_account_id = $1;

-- name: GetAccountUUIDByShortID :one
SELECT id FROM accounts WHERE short_id = $1;

-- name: ListAccounts :many
SELECT a.*,
  bc.short_id AS connection_short_id,
  bc.institution_name,
  u.short_id AS user_short_id,
  bc.status as connection_status,
  ow.short_id AS owner_user_short_id,
  ow.name AS owner_user_name
FROM accounts a
LEFT JOIN bank_connections bc ON a.connection_id = bc.id
LEFT JOIN users u ON u.id = bc.user_id
LEFT JOIN users ow ON ow.id = a.owner_user_id
ORDER BY bc.institution_name, a.name;

-- name: ListAccountsByUser :many
-- Effective-owner filter: an account belongs to a user when its per-account
-- owner override (if set) or, failing that, the connection owner matches.
SELECT a.*,
  bc.short_id AS connection_short_id,
  bc.institution_name,
  u.short_id AS user_short_id,
  bc.status as connection_status,
  ow.short_id AS owner_user_short_id,
  ow.name AS owner_user_name
FROM accounts a
LEFT JOIN bank_connections bc ON a.connection_id = bc.id
LEFT JOIN users u ON u.id = bc.user_id
LEFT JOIN users ow ON ow.id = a.owner_user_id
WHERE COALESCE(a.owner_user_id, bc.user_id) = $1 ORDER BY bc.institution_name, a.name;

-- name: GetAccount :one
SELECT a.*,
  bc.short_id AS connection_short_id,
  bc.institution_name,
  u.short_id AS user_short_id,
  bc.status as connection_status,
  ow.short_id AS owner_user_short_id,
  ow.name AS owner_user_name
FROM accounts a
LEFT JOIN bank_connections bc ON a.connection_id = bc.id
LEFT JOIN users u ON u.id = bc.user_id
LEFT JOIN users ow ON ow.id = a.owner_user_id
WHERE a.id = $1;

-- name: UpdateAccountDisplayName :one
UPDATE accounts SET display_name = $2, updated_at = NOW()
WHERE id = $1 RETURNING *;

-- name: UpdateAccountExcluded :one
UPDATE accounts SET excluded = $2, updated_at = NOW()
WHERE id = $1 RETURNING *;

-- name: UpdateAccountOwner :one
-- Sets (or clears, when $2 IS NULL) the per-account owner override. NULL means
-- the account inherits its bank connection's owner.
UPDATE accounts SET owner_user_id = $2, updated_at = NOW()
WHERE id = $1 RETURNING *;

-- name: ListExcludedAccountIDsByConnection :many
SELECT id FROM accounts WHERE connection_id = $1 AND excluded = true;

-- name: GetAccountDisplayNameByID :one
SELECT COALESCE(display_name, name) AS display_name FROM accounts WHERE id = $1;
