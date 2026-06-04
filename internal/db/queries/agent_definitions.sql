-- name: CreateAgentDefinition :one
INSERT INTO workflows (
    name, slug, prompt, system_prompt, schedule_cron,
    tool_scope, allowed_tools, model, max_turns, max_budget_usd, enabled,
    quiet_hours_start, quiet_hours_end, trigger_on_sync_complete, source_template,
    connectors
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING *;

-- name: GetAgentDefinition :one
SELECT * FROM workflows WHERE id = $1;

-- name: GetAgentDefinitionByShortID :one
SELECT * FROM workflows WHERE short_id = $1;

-- name: GetAgentDefinitionBySlug :one
SELECT * FROM workflows WHERE slug = $1;

-- name: GetWorkflowAvatarSeedBySlug :one
-- Resolves a workflow's custom DiceBear avatar seed from its stable slug.
-- NULL/absent means the avatar falls back to seeding on the slug itself.
-- Used by the avatar handler to render agent identicons.
SELECT avatar_seed FROM workflows WHERE slug = $1;

-- name: ListAgentDefinitions :many
SELECT * FROM workflows
ORDER BY created_at DESC;

-- name: ListEnabledAgentDefinitions :many
SELECT * FROM workflows
WHERE enabled = TRUE
ORDER BY created_at DESC;

-- name: UpdateAgentDefinition :one
UPDATE workflows
SET name                     = $2,
    slug                     = $3,
    prompt                   = $4,
    system_prompt            = $5,
    schedule_cron            = $6,
    tool_scope               = $7,
    allowed_tools            = $8,
    model                    = $9,
    max_turns                = $10,
    max_budget_usd           = $11,
    enabled                  = $12,
    quiet_hours_start        = $13,
    quiet_hours_end          = $14,
    trigger_on_sync_complete = $15,
    avatar_seed              = $16,
    connectors               = $17,
    updated_at               = NOW()
WHERE id = $1
RETURNING *;

-- name: ListAgentDefinitionsForSyncWebhook :many
-- Used by the post-sync hook to find agents that should fire after a
-- successful sync. Filtered by the partial index for cheap lookup.
SELECT * FROM workflows
WHERE enabled = TRUE AND trigger_on_sync_complete = TRUE
ORDER BY created_at DESC;

-- name: SetAgentDefinitionEnabled :one
UPDATE workflows
SET enabled    = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteAgentDefinition :execrows
DELETE FROM workflows WHERE id = $1;
