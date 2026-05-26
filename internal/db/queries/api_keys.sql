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

-- name: CleanupOrphanedAgentApiKeys :execresult
-- Reaps `actor_type='agent'` API keys that were minted for one run, never
-- revoked (orchestrator crashed / SIGKILL'd mid-run before its deferred
-- revoke could fire), and are old enough that the run could not still be
-- in flight. Run on serve startup. Idempotent.
--
-- 1 hour grace covers any pathological long-running agent without risk —
-- legitimate runs cap at agent.max_budget_usd and agent.max_turns anyway,
-- and the orchestrator wraps each run in a 30-minute hard timeout (see
-- RunNowAsyncWith).
UPDATE api_keys
SET revoked_at = NOW()
WHERE actor_type = 'agent'
  AND revoked_at IS NULL
  AND created_at < NOW() - INTERVAL '1 hour';
