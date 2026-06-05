-- name: CreateSyncSchedule :one
INSERT INTO sync_schedules (name, cron, preset, applies_to_all, enabled)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetSyncSchedule :one
SELECT * FROM sync_schedules WHERE id = $1;

-- name: GetSyncScheduleByShortID :one
SELECT * FROM sync_schedules WHERE short_id = $1;

-- name: ListSyncSchedules :many
SELECT
    s.*,
    s.applies_to_all OR EXISTS (
        SELECT 1 FROM sync_schedule_connections sc WHERE sc.schedule_id = s.id
    ) AS has_targets,
    (SELECT COUNT(*) FROM sync_schedule_connections sc WHERE sc.schedule_id = s.id) AS connection_count
FROM sync_schedules s
ORDER BY s.created_at ASC;

-- name: ListEnabledSyncSchedules :many
SELECT id, cron, applies_to_all FROM sync_schedules WHERE enabled = true;

-- name: ListSyncScheduleConnectionPairs :many
SELECT schedule_id, connection_id FROM sync_schedule_connections;

-- name: UpdateSyncSchedule :one
UPDATE sync_schedules
SET name = $2, cron = $3, preset = $4, applies_to_all = $5, enabled = $6, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetSyncScheduleEnabled :one
UPDATE sync_schedules SET enabled = $2, updated_at = NOW()
WHERE id = $1 RETURNING *;

-- name: DeleteSyncSchedule :exec
DELETE FROM sync_schedules WHERE id = $1;

-- name: ListConnectionIDsForSchedule :many
SELECT connection_id FROM sync_schedule_connections WHERE schedule_id = $1;

-- name: ListConnectionShortIDsForSchedule :many
SELECT bc.short_id
FROM sync_schedule_connections sc
JOIN bank_connections bc ON bc.id = sc.connection_id
WHERE sc.schedule_id = $1
ORDER BY bc.created_at;

-- name: AddScheduleConnection :exec
INSERT INTO sync_schedule_connections (schedule_id, connection_id)
VALUES ($1, $2)
ON CONFLICT (schedule_id, connection_id) DO NOTHING;

-- name: ClearScheduleConnections :exec
DELETE FROM sync_schedule_connections WHERE schedule_id = $1;

-- name: ListSchedulesForConnection :many
SELECT s.*
FROM sync_schedules s
WHERE s.enabled = true
  AND (
    s.applies_to_all
    OR EXISTS (
      SELECT 1 FROM sync_schedule_connections sc
      WHERE sc.schedule_id = s.id AND sc.connection_id = $1
    )
  )
ORDER BY s.created_at ASC;
