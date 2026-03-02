-- name: CreateApiKey :one
INSERT INTO api_keys (name, key_hash, key_prefix) VALUES ($1, $2, $3) RETURNING *;

-- name: GetApiKeyByHash :one
SELECT * FROM api_keys WHERE key_hash = $1;

-- name: ListApiKeys :many
SELECT * FROM api_keys ORDER BY created_at DESC;

-- name: RevokeApiKey :exec
UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL;

-- name: GetApiKey :one
SELECT * FROM api_keys WHERE id = $1;

-- name: UpdateApiKeyLastUsed :exec
UPDATE api_keys SET last_used_at = NOW() WHERE id = $1;
