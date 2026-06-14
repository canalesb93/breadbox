-- name: CreateAgentReport :one
INSERT INTO agent_reports (title, body, created_by_type, created_by_id, created_by_name, priority, tags, author, session_id, workflow_run_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetAgentReport :one
SELECT * FROM agent_reports WHERE id = $1;

-- name: GetAgentReportUUIDByShortID :one
SELECT id FROM agent_reports WHERE short_id = $1;

-- name: ListAgentReports :many
SELECT * FROM agent_reports ORDER BY created_at DESC LIMIT $1;

-- name: ListUnreadAgentReports :many
SELECT * FROM agent_reports WHERE read_at IS NULL ORDER BY created_at DESC LIMIT $1;

-- name: CountUnreadAgentReports :one
SELECT COUNT(*) FROM agent_reports WHERE read_at IS NULL;

-- name: MarkAgentReportRead :exec
UPDATE agent_reports SET read_at = NOW() WHERE id = $1 AND read_at IS NULL;

-- name: MarkAgentReportUnread :exec
UPDATE agent_reports SET read_at = NULL WHERE id = $1;

-- name: MarkAllAgentReportsRead :exec
UPDATE agent_reports SET read_at = NOW() WHERE read_at IS NULL;

-- name: DeleteAgentReport :exec
DELETE FROM agent_reports WHERE id = $1;

-- name: ListReportSummariesForRunIDs :many
-- Returns short report summaries for a batch of workflow_runs UUIDs.
-- Powers the AgentRunRow chip on the runs landing — operators see
-- "this run produced these reports" at a glance.
SELECT
    workflow_run_id,
    id,
    short_id,
    title,
    priority,
    created_at
FROM agent_reports
WHERE workflow_run_id = ANY($1::uuid[])
ORDER BY created_at ASC;
