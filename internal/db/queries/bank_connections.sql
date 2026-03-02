-- name: CreateBankConnection :one
INSERT INTO bank_connections (provider, institution_id, institution_name, plaid_item_id, plaid_access_token, status, user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetBankConnection :one
SELECT bc.*, u.name as user_name
FROM bank_connections bc
LEFT JOIN users u ON bc.user_id = u.id
WHERE bc.id = $1;

-- name: ListBankConnections :many
SELECT bc.*, u.name as user_name,
  (SELECT COUNT(*) FROM accounts a WHERE a.connection_id = bc.id) as account_count
FROM bank_connections bc
LEFT JOIN users u ON bc.user_id = u.id
ORDER BY bc.created_at DESC;

-- name: UpdateBankConnectionStatus :exec
UPDATE bank_connections
SET status = $2, error_code = $3, error_message = $4, updated_at = NOW()
WHERE id = $1;

-- name: UpdateBankConnectionCursor :exec
UPDATE bank_connections
SET sync_cursor = $2, last_synced_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: DeleteBankConnection :exec
UPDATE bank_connections
SET status = 'disconnected', plaid_access_token = NULL, updated_at = NOW()
WHERE id = $1;

-- name: GetBankConnectionByPlaidItemID :one
SELECT * FROM bank_connections WHERE plaid_item_id = $1;

-- name: CountConnectionsNeedingAttention :one
SELECT COUNT(*) FROM bank_connections WHERE status IN ('error', 'pending_reauth');

-- name: ListActiveConnections :many
SELECT * FROM bank_connections WHERE status = 'active';

-- name: GetBankConnectionForSync :one
SELECT id, provider, plaid_item_id, plaid_access_token, sync_cursor, user_id FROM bank_connections WHERE id = $1 AND status != 'disconnected';

-- name: ListConnectionsForAPI :many
SELECT bc.id, bc.user_id, bc.provider, bc.institution_id, bc.institution_name,
       bc.status, bc.error_code, bc.error_message, bc.last_synced_at,
       bc.created_at, bc.updated_at, u.name as user_name
FROM bank_connections bc LEFT JOIN users u ON bc.user_id = u.id
WHERE bc.status != 'disconnected' ORDER BY bc.created_at;

-- name: ListConnectionsByUserForAPI :many
SELECT bc.id, bc.user_id, bc.provider, bc.institution_id, bc.institution_name,
       bc.status, bc.error_code, bc.error_message, bc.last_synced_at,
       bc.created_at, bc.updated_at, u.name as user_name
FROM bank_connections bc LEFT JOIN users u ON bc.user_id = u.id
WHERE bc.user_id = $1 AND bc.status != 'disconnected' ORDER BY bc.created_at;

-- name: GetConnectionForAPI :one
SELECT bc.id, bc.user_id, bc.provider, bc.institution_id, bc.institution_name,
       bc.status, bc.error_code, bc.error_message, bc.last_synced_at,
       bc.created_at, bc.updated_at, u.name as user_name
FROM bank_connections bc LEFT JOIN users u ON bc.user_id = u.id
WHERE bc.id = $1;
