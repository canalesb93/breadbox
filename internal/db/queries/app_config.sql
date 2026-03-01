-- name: GetAppConfig :one
SELECT key, value, updated_at FROM app_config WHERE key = $1;

-- name: ListAppConfig :many
SELECT key, value, updated_at FROM app_config ORDER BY key;

-- name: SetAppConfig :exec
INSERT INTO app_config (key, value, updated_at)
VALUES ($1, $2, NOW())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();
