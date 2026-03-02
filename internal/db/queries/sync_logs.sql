-- name: CreateSyncLog :one
INSERT INTO sync_logs (connection_id, "trigger", status, started_at, completed_at, added_count, modified_count, removed_count, error_message)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListRecentSyncLogs :many
SELECT sl.*, bc.institution_name
FROM sync_logs sl
JOIN bank_connections bc ON sl.connection_id = bc.id
ORDER BY sl.started_at DESC
LIMIT 5;

-- name: GetLastSuccessfulSyncTime :one
SELECT MAX(completed_at)::timestamptz as last_sync_time
FROM sync_logs
WHERE status = 'success';

-- name: GetSyncLogsByConnection :many
SELECT * FROM sync_logs
WHERE connection_id = $1
ORDER BY started_at DESC
LIMIT $2 OFFSET $3;
