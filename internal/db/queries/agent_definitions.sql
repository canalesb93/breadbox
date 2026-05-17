-- name: CreateAgentDefinition :one
INSERT INTO agent_definitions (
    name, slug, prompt, system_prompt, schedule_cron,
    tool_scope, allowed_tools, model, max_turns, max_budget_usd, enabled
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetAgentDefinition :one
SELECT * FROM agent_definitions WHERE id = $1;

-- name: GetAgentDefinitionByShortID :one
SELECT * FROM agent_definitions WHERE short_id = $1;

-- name: GetAgentDefinitionBySlug :one
SELECT * FROM agent_definitions WHERE slug = $1;

-- name: ListAgentDefinitions :many
SELECT * FROM agent_definitions
ORDER BY created_at DESC;

-- name: ListEnabledAgentDefinitions :many
SELECT * FROM agent_definitions
WHERE enabled = TRUE
ORDER BY created_at DESC;

-- name: UpdateAgentDefinition :one
UPDATE agent_definitions
SET name           = $2,
    slug           = $3,
    prompt         = $4,
    system_prompt  = $5,
    schedule_cron  = $6,
    tool_scope     = $7,
    allowed_tools  = $8,
    model          = $9,
    max_turns      = $10,
    max_budget_usd = $11,
    enabled        = $12,
    updated_at     = NOW()
WHERE id = $1
RETURNING *;

-- name: SetAgentDefinitionEnabled :one
UPDATE agent_definitions
SET enabled    = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteAgentDefinition :execrows
DELETE FROM agent_definitions WHERE id = $1;
