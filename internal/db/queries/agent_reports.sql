-- name: CreateAgentReport :one
INSERT INTO agent_reports (title, body, created_by_type, created_by_id, created_by_name, priority, tags, author)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetAgentReport :one
SELECT * FROM agent_reports WHERE id = $1;

-- name: ListAgentReports :many
SELECT * FROM agent_reports ORDER BY created_at DESC LIMIT $1;

-- name: ListUnreadAgentReports :many
SELECT * FROM agent_reports WHERE read_at IS NULL ORDER BY created_at DESC LIMIT $1;

-- name: CountUnreadAgentReports :one
SELECT COUNT(*) FROM agent_reports WHERE read_at IS NULL;

-- name: MarkAgentReportRead :exec
UPDATE agent_reports SET read_at = NOW() WHERE id = $1 AND read_at IS NULL;

-- name: MarkAllAgentReportsRead :exec
UPDATE agent_reports SET read_at = NOW() WHERE read_at IS NULL;
