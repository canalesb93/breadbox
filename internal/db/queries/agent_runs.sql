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
    session_id            = $13,
    error_message         = $14
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

-- name: GetAgentRecentCapStats :many
-- Per-definition "is this agent regularly hitting its safety ceilings?"
-- signal: count of runs in the last 5 non-skipped that had hit_cap set
-- (either max_turns OR max_budget). Mirrors GetAgentRecentErrorStats —
-- the admin list page renders a warning pill at 2+ recent cap hits so
-- the operator knows to raise max_turns / max_budget_usd or split the
-- prompt.
WITH ranked AS (
    SELECT agent_definition_id, hit_cap,
           ROW_NUMBER() OVER (
               PARTITION BY agent_definition_id
               ORDER BY started_at DESC
           ) AS rn
    FROM agent_runs
    WHERE agent_definition_id IS NOT NULL
      AND status != 'skipped'
)
SELECT agent_definition_id,
       COUNT(*) FILTER (WHERE hit_cap IS NOT NULL)::int AS cap_count,
       COUNT(*)::int                                     AS run_count
FROM ranked
WHERE rn <= 5
GROUP BY agent_definition_id;

-- name: GetAgentRecentErrorStats :many
-- Per-definition "is this agent broken right now?" signal: error count
-- in the last 5 non-skipped runs. Skipped runs (quiet hours, concurrency
-- lock) are excluded — they aren't agent failures. The admin list
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
-- Per-definition cost rollup over the last 30 days. Used by the admin
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

-- name: ListRecentErroredAgentRuns :many
-- Surfaces the most recent errored runs across all agents in the last
-- N hours, joined with agent slug + name for the admin "Run-failed
-- banner" on the /agents list page. Bounded by the LIMIT — the banner shows at
-- most 5 entries, but we ask for a few extras so the UI can say
-- "showing 5 of N most recent errors" without a second query.
SELECT r.short_id      AS run_short_id,
       r.started_at    AS started_at,
       r.error_message AS error_message,
       r.duration_ms   AS duration_ms,
       r.hit_cap       AS hit_cap,
       d.slug          AS agent_slug,
       d.name          AS agent_name
FROM agent_runs r
JOIN agent_definitions d ON d.id = r.agent_definition_id
WHERE r.status = 'error'
  AND r.started_at >= NOW() - ($1::int * INTERVAL '1 hour')
ORDER BY r.started_at DESC
LIMIT $2::int;

-- name: SetAgentRunPromptPrefix :exec
-- Set the operator-supplied prompt prefix for a "run now" trigger. Called
-- immediately after CreateAgentRun + before AssembleJobSpec so the prefix
-- is captured in the audit trail even if the run fails to assemble.
UPDATE agent_runs
SET prompt_prefix = $2
WHERE id = $1;

-- name: SetAgentRunHitCap :one
-- Record which safety cap (if any) terminated the run. Called by the
-- orchestrator immediately after CompleteAgentRun when the sidecar signaled
-- max_turns or budget_exceeded via the returned RunResult error. Returns
-- the updated row so AgentRunFromRow can rebuild the response with the
-- new field populated.
UPDATE agent_runs
SET hit_cap = $2
WHERE id = $1
RETURNING *;

-- name: GetAgentLastPromptPrefixes :many
-- Per-definition most recent non-null prompt_prefix. Skipped + null-prefix
-- rows don't shadow earlier prefixes — only runs that actually carried a
-- prefix are eligible. Used by the admin "Use last prefix" button on the
-- Run now dialog to pre-fill the operator's prior context.
SELECT DISTINCT ON (agent_definition_id)
       agent_definition_id, prompt_prefix
FROM agent_runs
WHERE agent_definition_id IS NOT NULL
  AND prompt_prefix IS NOT NULL
  AND prompt_prefix <> ''
ORDER BY agent_definition_id, started_at DESC;

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

-- name: GetAgentRunsExtraStats30d :one
-- Cross-agent 30-day rollup of error count + average duration. Powers
-- the StatTile row on the /agents landing. Skipped runs (quiet hours,
-- concurrency lock) are excluded from the duration average — they
-- aren't real workload and distort it — but are *not* excluded from
-- the error count (which already filters on status = 'error').
SELECT
    COUNT(*) FILTER (WHERE status = 'error')::int        AS error_count,
    (COALESCE(AVG(duration_ms) FILTER (
        WHERE status != 'skipped' AND duration_ms IS NOT NULL
    ), 0)::float8 / 1000.0)::float8                      AS avg_duration_seconds
FROM agent_runs
WHERE started_at >= NOW() - INTERVAL '30 days';

-- name: GetAgentLifetimeStats :one
-- Per-agent lifetime rollup powering the StatTile row on the agent
-- detail page (/agents/{slug}). Includes total runs, errors, total
-- cost, and average duration. Skipped runs (quiet hours, concurrency
-- lock) are excluded from the cost + duration aggregates since they
-- represent no real workload; they ARE counted in run_count so the
-- operator sees the full history. Returns a single row (with zeros)
-- even when the agent has no runs yet.
SELECT
    COUNT(*)::int                                          AS run_count,
    COUNT(*) FILTER (WHERE status = 'skipped')::int        AS skipped_count,
    COUNT(*) FILTER (WHERE status = 'error')::int          AS error_count,
    COALESCE(
        SUM(total_cost_usd) FILTER (WHERE status != 'skipped'),
        0
    )::numeric(12,4)                                       AS total_cost_usd,
    (COALESCE(AVG(duration_ms) FILTER (
        WHERE status != 'skipped' AND duration_ms IS NOT NULL
    ), 0)::float8 / 1000.0)::float8                        AS avg_duration_seconds
FROM agent_runs
WHERE agent_definition_id = $1;

-- GetHouseholdCostSince sums total_cost_usd across ALL agent definitions
-- for runs started at/after the given instant, excluding skipped rows.
-- Powers the household spend-ceiling gate + the settings spend display.
-- name: GetHouseholdCostSince :one
SELECT COALESCE(SUM(total_cost_usd), 0)::numeric(12,4) AS total_cost_usd
FROM agent_runs
WHERE started_at >= sqlc.arg(since)
  AND status != 'skipped';
