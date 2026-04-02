-- name: CreateMCPSession :one
INSERT INTO mcp_sessions (api_key_id, api_key_name, purpose)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetMCPSessionByID :one
SELECT * FROM mcp_sessions WHERE id = $1;

-- name: GetMCPSessionByShortID :one
SELECT * FROM mcp_sessions WHERE short_id = $1;

-- name: ListMCPSessions :many
SELECT
    ms.*,
    COALESCE(tc.call_count, 0)::bigint AS tool_call_count,
    tc.last_call_at::timestamptz AS last_call_at,
    COALESCE(ar.author, ar.created_by_name, '')::text AS agent_name,
    ar.id AS report_id,
    COALESCE(ar.title, '')::text AS report_title
FROM mcp_sessions ms
LEFT JOIN LATERAL (
    SELECT COUNT(*) AS call_count, MAX(created_at) AS last_call_at
    FROM mcp_tool_calls WHERE session_id = ms.id
) tc ON true
LEFT JOIN LATERAL (
    SELECT id, author, created_by_name, title
    FROM agent_reports WHERE session_id = ms.id
    ORDER BY created_at DESC LIMIT 1
) ar ON true
ORDER BY ms.created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountMCPSessions :one
SELECT COUNT(*) FROM mcp_sessions;

-- name: CreateToolCallLog :exec
INSERT INTO mcp_tool_calls (session_id, tool_name, classification, reason, request_json, response_json, is_error, actor_type, actor_id, actor_name, duration_ms)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: ListToolCallsBySession :many
SELECT * FROM mcp_tool_calls
WHERE session_id = $1
ORDER BY created_at ASC;
