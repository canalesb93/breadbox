-- name: CreateApiKey :one
INSERT INTO api_keys (name, key_hash, key_prefix, scope, actor_type, actor_name)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetApiKeyByHash :one
SELECT * FROM api_keys WHERE key_hash = $1;

-- name: GetApiKeyByPrefix :one
SELECT * FROM api_keys WHERE key_prefix = $1 LIMIT 1;

-- name: ListApiKeys :many
SELECT * FROM api_keys ORDER BY created_at DESC;

-- name: RevokeApiKey :exec
UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL;

-- name: GetApiKey :one
SELECT * FROM api_keys WHERE id = $1;

-- name: UpdateApiKeyLastUsed :exec
UPDATE api_keys SET last_used_at = NOW() WHERE id = $1;

-- name: CountActiveApiKeys :one
SELECT COUNT(*) FROM api_keys WHERE revoked_at IS NULL;
