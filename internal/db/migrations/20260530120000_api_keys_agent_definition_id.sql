-- +goose Up
-- Link a minted per-run agent API key to the agent_definition it runs
-- for, so every piece of an agent's activity can resolve to ONE
-- canonical identity (the definition's name + a slug-seeded avatar)
-- instead of the generic MCP clientInfo ("claude-code") the Claude
-- Agent SDK presents on every connection.
--
-- Nullable + ON DELETE SET NULL: human/system keys and the auto-managed
-- mcp-client identities carry no definition; deleting a definition
-- preserves its historical run keys (they fall back to slug-from-name
-- resolution at render time).
ALTER TABLE api_keys
    ADD COLUMN agent_definition_id UUID NULL REFERENCES agent_definitions(id) ON DELETE SET NULL;

CREATE INDEX idx_api_keys_agent_definition_id
    ON api_keys (agent_definition_id)
    WHERE agent_definition_id IS NOT NULL;

-- Backfill existing per-run keys (name format `agent:<slug>:<runShortID>`)
-- by matching the embedded slug to agent_definitions.slug. Best-effort;
-- keys whose slug no longer maps to a live definition stay NULL.
UPDATE api_keys ak
SET agent_definition_id = ad.id
FROM agent_definitions ad
WHERE ak.actor_type = 'agent'
  AND ak.name LIKE 'agent:%:%'
  AND ad.slug = SPLIT_PART(ak.name, ':', 2)
  AND ak.agent_definition_id IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_api_keys_agent_definition_id;
ALTER TABLE api_keys DROP COLUMN IF EXISTS agent_definition_id;
