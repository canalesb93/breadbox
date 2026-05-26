-- +goose Up
-- Adds a client fingerprint column to api_keys so the MCP server can
-- auto-create stable per-client agent identities (one row per distinct
-- MCP client name+host+transport) from the clientInfo block sent on
-- the `initialize` handshake.
--
-- HTTP MCP api_keys (user-created credentials) leave this NULL — the
-- partial unique index doesn't include them. Auto-managed MCP-client
-- rows set it to `lower(coalesce(title, name)) + "@" + lower(host) + "@" + transport`
-- (host falls back to "" when no websiteURL is present). The index
-- guarantees we never create two rows for the same fingerprint, even
-- under a concurrent first-call race.
--
-- The fallback "Local MCP" row (relabelled from the old stdio
-- singleton) carries `unknown@@stdio` so the same EnsureMCPClientAgentKey
-- lookup-then-insert path returns it for `initialize` requests that
-- omit clientInfo entirely.
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS client_fingerprint TEXT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS api_keys_client_fingerprint_idx
    ON api_keys (client_fingerprint)
    WHERE client_fingerprint IS NOT NULL;

-- Relabel the existing stdio singleton in-place. It was created with
-- actor_type='system' and actor_name='stdio'; with the per-client
-- lookup landing it becomes the no-clientInfo fallback identity, so
-- it's now an agent named "Local MCP" with the reserved fingerprint
-- `unknown@@stdio`. Old annotations.actor_id rows pointing at this
-- UUID render correctly without any data backfill — they re-resolve
-- through the same row, which is now an agent.
UPDATE api_keys
SET    actor_type = 'agent',
       actor_name = 'Local MCP',
       client_fingerprint = 'unknown@@stdio',
       name = 'mcp-client:unknown@@stdio'
WHERE  key_prefix = 'bb_stdio_singleton'
  AND  actor_type = 'system';

-- +goose Down
-- Roll back the relabel first so we don't leave an orphan
-- client_fingerprint when the column is dropped.
UPDATE api_keys
SET    actor_type = 'system',
       actor_name = 'stdio',
       client_fingerprint = NULL,
       name = 'MCP Stdio'
WHERE  key_prefix = 'bb_stdio_singleton'
  AND  client_fingerprint = 'unknown@@stdio';

DROP INDEX IF EXISTS api_keys_client_fingerprint_idx;

ALTER TABLE api_keys
    DROP COLUMN IF EXISTS client_fingerprint;
