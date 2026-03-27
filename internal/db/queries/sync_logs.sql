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

-- name: UpdateSyncLog :exec
UPDATE sync_logs
SET status = $2, completed_at = $3, added_count = $4, modified_count = $5, removed_count = $6, error_message = $7, duration_ms = $8, warning_message = $9
WHERE id = $1;

-- name: GetMostRecentSyncLog :one
SELECT * FROM sync_logs WHERE connection_id = $1 ORDER BY started_at DESC LIMIT 1;

-- name: CleanupOrphanedSyncLogs :execresult
UPDATE sync_logs SET status = 'error', error_message = 'interrupted by server restart', completed_at = NOW()
WHERE status = 'in_progress';

-- name: DeleteSyncLogsOlderThan :execresult
DELETE FROM sync_logs
WHERE started_at < $1
  AND status != 'in_progress';

-- name: CountSyncLogs :one
SELECT COUNT(*) FROM sync_logs;
