-- name: CreateBankConnection :one
INSERT INTO bank_connections (provider, institution_id, institution_name, external_id, encrypted_credentials, status, user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetBankConnection :one
SELECT bc.*, u.name as user_name
FROM bank_connections bc
LEFT JOIN users u ON bc.user_id = u.id
WHERE bc.id = $1;

-- name: GetConnectionUUIDByShortID :one
SELECT id FROM bank_connections WHERE short_id = $1;

-- name: ListBankConnections :many
SELECT bc.*, u.name as user_name,
  (SELECT COUNT(*) FROM accounts a WHERE a.connection_id = bc.id) as account_count,
  COALESCE(ls.status::text, '')::text as last_sync_status,
  COALESCE(ls.trigger::text, '')::text as last_sync_trigger,
  COALESCE(ls.duration_ms, 0) as last_sync_duration_ms,
  ls.started_at as last_sync_started_at,
  COALESCE(ls.added_count, 0) as last_sync_added,
  COALESCE(ls.modified_count, 0) as last_sync_modified,
  COALESCE(ls.removed_count, 0) as last_sync_removed,
  ls.error_message as last_sync_error_message
FROM bank_connections bc
LEFT JOIN users u ON bc.user_id = u.id
LEFT JOIN LATERAL (
  SELECT sl.status, sl.trigger, sl.duration_ms, sl.started_at,
         sl.added_count, sl.modified_count, sl.removed_count, sl.error_message
  FROM sync_logs sl
  WHERE sl.connection_id = bc.id
  ORDER BY sl.started_at DESC
  LIMIT 1
) ls ON true
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
SET status = 'disconnected', encrypted_credentials = NULL, updated_at = NOW()
WHERE id = $1;

-- name: GetBankConnectionByExternalID :one
SELECT * FROM bank_connections WHERE provider = $1 AND external_id = $2;

-- name: CountConnectionsNeedingAttention :one
SELECT COUNT(*) FROM bank_connections WHERE status IN ('error', 'pending_reauth');

-- name: ListActiveConnections :many
SELECT * FROM bank_connections WHERE status = 'active';

-- name: GetBankConnectionForSync :one
SELECT id, provider, external_id, encrypted_credentials, sync_cursor, user_id FROM bank_connections WHERE id = $1 AND status != 'disconnected';

-- name: ListConnectionsForAPI :many
SELECT bc.id, bc.short_id, bc.user_id, bc.provider, bc.institution_id, bc.institution_name,
       bc.status, bc.error_code, bc.error_message, bc.last_synced_at,
       bc.created_at, bc.updated_at, u.name as user_name
FROM bank_connections bc LEFT JOIN users u ON bc.user_id = u.id
WHERE bc.status != 'disconnected' ORDER BY bc.created_at;

-- name: ListConnectionsByUserForAPI :many
SELECT bc.id, bc.short_id, bc.user_id, bc.provider, bc.institution_id, bc.institution_name,
       bc.status, bc.error_code, bc.error_message, bc.last_synced_at,
       bc.created_at, bc.updated_at, u.name as user_name
FROM bank_connections bc LEFT JOIN users u ON bc.user_id = u.id
WHERE bc.user_id = $1 AND bc.status != 'disconnected' ORDER BY bc.created_at;

-- name: GetConnectionForAPI :one
SELECT bc.id, bc.short_id, bc.user_id, bc.provider, bc.institution_id, bc.institution_name,
       bc.status, bc.error_code, bc.error_message, bc.last_synced_at,
       bc.created_at, bc.updated_at, u.name as user_name
FROM bank_connections bc LEFT JOIN users u ON bc.user_id = u.id
WHERE bc.id = $1;

-- name: UpdateConnectionNewAccounts :exec
UPDATE bank_connections SET new_accounts_available = $2, updated_at = NOW() WHERE id = $1;

-- name: UpdateConnectionConsentExpiration :exec
UPDATE bank_connections SET consent_expiration_time = $2, status = 'pending_reauth', updated_at = NOW() WHERE id = $1;

-- name: UpdateConnectionPaused :one
UPDATE bank_connections SET paused = $2, updated_at = NOW()
WHERE id = $1 RETURNING *;

-- name: UpdateConnectionSyncInterval :one
UPDATE bank_connections SET sync_interval_override_minutes = $2, updated_at = NOW()
WHERE id = $1 RETURNING *;

-- name: ListActiveUnpausedConnections :many
SELECT * FROM bank_connections WHERE status = 'active' AND paused = false;

-- name: CountConnectionsByUserID :many
SELECT user_id, COUNT(*) as connection_count
FROM bank_connections
WHERE status != 'disconnected'
GROUP BY user_id;

-- name: IncrementConsecutiveFailures :exec
UPDATE bank_connections
SET consecutive_failures = consecutive_failures + 1, last_error_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: ResetConsecutiveFailures :exec
UPDATE bank_connections
SET consecutive_failures = 0, updated_at = NOW()
WHERE id = $1;

-- name: CountConnections :one
SELECT count(*) FROM bank_connections WHERE status != 'disconnected';
