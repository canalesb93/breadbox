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
-- Returns only user-manageable credentials. Drives /settings/api-keys, the
-- REST GET /api/v1/api-keys list, and `breadbox keys list`.
--
-- Hides agent machinery — keys a user neither created nor can act on — but
-- identifies it STRUCTURALLY, never by actor_type. The actor_type column was
-- added with DEFAULT 'agent' (migration 20260512061200), so every key minted
-- before that migration — including legitimate user keys — carries
-- actor_type='agent'. Filtering on `actor_type <> 'agent'` would make those
-- still-valid user credentials vanish from every management surface while
-- staying live and un-revocable. So we match the two positively-identifiable
-- machine shapes instead:
--   * client_fingerprint IS NOT NULL — auto-managed MCP-client identity rows
--     (CreateMCPClientApiKey), which anchor per-client agent avatars.
--   * per-run keys minted by Orchestrator.MintRunAPIKey, identified by a set
--     workflow_id (modern) OR the `agent:<slug>:<runID>` name (run keys minted
--     before workflow_id existed). The name match mirrors ParseAgentKeySlug.
-- These churned on every workflow run — flickering into the list mid-run and
-- piling up under "revoked" — which is what looked like unexplained key
-- activity. Their identity + run history surface on the Workflows / agent-runs
-- pages; the avatar resolver still reads the rows by id (ResolveAgentSlugForActor),
-- so they're hidden from management, not deleted.
--
-- Everything else — user keys (any name), the `system` stdio bootstrap key, and
-- legacy user keys mislabeled actor_type='agent' by the DEFAULT — stays visible.
SELECT * FROM api_keys
WHERE client_fingerprint IS NULL
  AND workflow_id IS NULL
  AND name NOT LIKE 'agent:%:%'
ORDER BY created_at DESC;

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
