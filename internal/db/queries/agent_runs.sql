-- name: CreateAgentRun :one
INSERT INTO agent_runs (agent_definition_id, "trigger", status, started_at)
VALUES ($1, $2, 'in_progress', NOW())
RETURNING *;

-- name: GetAgentRun :one
SELECT * FROM agent_runs WHERE id = $1;

-- name: GetAgentRunByShortID :one
SELECT * FROM agent_runs WHERE short_id = $1;

-- name: GetLatestAgentRun :one
SELECT * FROM agent_runs
WHERE agent_definition_id = $1
ORDER BY started_at DESC
LIMIT 1;

-- name: ListAgentRuns :many
SELECT * FROM agent_runs
WHERE agent_definition_id = $1
ORDER BY started_at DESC
LIMIT $2 OFFSET $3;

-- name: ListRecentAgentRuns :many
SELECT * FROM agent_runs
ORDER BY started_at DESC
LIMIT $1 OFFSET $2;

-- name: CompleteAgentRun :one
UPDATE agent_runs
SET status                = $2,
    completed_at          = NOW(),
    duration_ms           = $3,
    total_cost_usd        = $4,
    input_tokens          = $5,
    output_tokens         = $6,
    cache_read_tokens     = $7,
    cache_creation_tokens = $8,
    turn_count            = $9,
    max_turns_used        = $10,
    num_tool_calls        = $11,
    transcript_path       = $12,
    session_id            = $13
WHERE id = $1
RETURNING *;

-- name: MarkAgentRunError :exec
UPDATE agent_runs
SET status          = 'error',
    completed_at    = NOW(),
    duration_ms     = (EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000)::INTEGER,
    error_message   = $2,
    transcript_path = $3
WHERE id = $1;

-- name: MarkAgentRunSkipped :exec
UPDATE agent_runs
SET status        = 'skipped',
    completed_at  = NOW(),
    error_message = $2
WHERE id = $1;

-- name: GetAgentRecentErrorStats :many
-- Per-definition "is this agent broken right now?" signal: error count
-- in the last 5 non-skipped runs. Skipped runs (quiet hours, concurrency
-- lock) are excluded — they aren't agent failures. The v2 SPA list
-- renders a warning pill when error_count >= 3.
WITH ranked AS (
    SELECT agent_definition_id, status,
           ROW_NUMBER() OVER (
               PARTITION BY agent_definition_id
               ORDER BY started_at DESC
           ) AS rn
    FROM agent_runs
    WHERE agent_definition_id IS NOT NULL
      AND status != 'skipped'
)
SELECT agent_definition_id,
       COUNT(*) FILTER (WHERE status = 'error')::int AS error_count,
       COUNT(*)::int                                  AS run_count
FROM ranked
WHERE rn <= 5
GROUP BY agent_definition_id;

-- name: GetAgentCostStats30d :many
-- Per-definition cost rollup over the last 30 days. Used by the v2 SPA
-- list page to surface lifetime spend at a glance. Excludes skipped runs
-- (no real cost incurred) but includes errored runs (often DO incur
-- partial cost).
SELECT
    agent_definition_id,
    COUNT(*)::int                                        AS run_count,
    COALESCE(SUM(total_cost_usd), 0)::numeric(10,4)      AS total_cost_usd
FROM agent_runs
WHERE agent_definition_id IS NOT NULL
  AND started_at >= NOW() - INTERVAL '30 days'
  AND status != 'skipped'
GROUP BY agent_definition_id;

-- name: SetAgentRunNote :one
UPDATE agent_runs
SET operator_note = $2
WHERE id = $1
RETURNING *;

-- name: SetAgentRunPromptPrefix :exec
-- Set the operator-supplied prompt prefix for a "run now" trigger. Called
-- immediately after CreateAgentRun + before AssembleJobSpec so the prefix
-- is captured in the audit trail even if the run fails to assemble.
UPDATE agent_runs
SET prompt_prefix = $2
WHERE id = $1;

-- name: CleanupOrphanedAgentRuns :execresult
UPDATE agent_runs
SET status        = 'error',
    error_message = 'interrupted by server restart',
    completed_at  = NOW()
WHERE status = 'in_progress';

-- name: DeleteAgentRunsOlderThan :execresult
DELETE FROM agent_runs
WHERE started_at < $1
  AND status != 'in_progress';

-- name: CountInProgressAgentRuns :one
SELECT COUNT(*) FROM agent_runs WHERE status = 'in_progress';
