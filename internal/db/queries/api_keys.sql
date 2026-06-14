-- name: CreateApiKey :one
INSERT INTO api_keys (name, key_hash, key_prefix, scope, actor_type, actor_name, workflow_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetApiKeyByHash :one
SELECT * FROM api_keys WHERE key_hash = $1;

-- name: GetAgentIdentityByApiKeyID :one
-- Canonical agent identity for an actor: the agent_definition linked to
-- the api_keys row at mint (per-run keys). Lets every surface render an
-- agent's activity under one name + slug-seeded avatar instead of the
-- MCP clientInfo the SDK presents. Returns no rows for non-agent keys
-- or run keys minted before workflow_id existed (callers fall
-- back to parsing the slug from the key name).
SELECT ad.id, ad.short_id, ad.name, ad.slug
FROM api_keys ak
JOIN workflows ad ON ad.id = ak.workflow_id
WHERE ak.id = $1;

-- name: GetApiKeyByPrefix :one
SELECT * FROM api_keys WHERE key_prefix = $1 LIMIT 1;

-- name: ListApiKeys :many
-- Filters out auto-managed MCP-client identities (client_fingerprint
-- IS NOT NULL). Those rows exist to anchor agent attribution + the
-- per-client avatar to a stable api_keys.id; they're not user-revocable
-- credentials and showing them in /settings/api-keys would confuse
-- users who don't recognise the "mcp-client:claude-desktop@@stdio"
-- prefix or have no way to act on the row.
SELECT * FROM api_keys WHERE client_fingerprint IS NULL ORDER BY created_at DESC;

-- name: GetApiKeyByClientFingerprint :one
-- Drives EnsureMCPClientAgentKey's lookup half. Returns the per-client
-- agent identity row, or pgx.ErrNoRows for the insert path.
SELECT * FROM api_keys WHERE client_fingerprint = $1 LIMIT 1;

-- name: CreateMCPClientApiKey :one
-- Auto-creates the per-client agent identity row on first contact
-- from a new MCP client. key_hash is a per-fingerprint sentinel that
-- never matches a presented SHA-256, so the row is identity-only —
-- it can never authenticate a real request.
INSERT INTO api_keys (name, key_hash, key_prefix, scope, actor_type, actor_name, client_fingerprint)
VALUES ($1, $2, $3, 'full_access', 'agent', $4, $5)
RETURNING *;

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
