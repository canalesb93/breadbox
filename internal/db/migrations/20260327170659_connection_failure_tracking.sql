-- +goose Up
ALTER TABLE bank_connections ADD COLUMN consecutive_failures INTEGER NOT NULL DEFAULT 0;
ALTER TABLE bank_connections ADD COLUMN last_error_at TIMESTAMPTZ NULL;

-- Backfill from most recent sync logs: count consecutive trailing errors per connection.
WITH ranked AS (
    SELECT connection_id, status,
           ROW_NUMBER() OVER (PARTITION BY connection_id ORDER BY started_at DESC) AS rn
    FROM sync_logs
),
streak AS (
    SELECT connection_id, COUNT(*) AS failures
    FROM ranked
    WHERE rn <= (
        SELECT COALESCE(MIN(r2.rn), 0) - 1
        FROM ranked r2
        WHERE r2.connection_id = ranked.connection_id AND r2.status != 'error'
    )
    AND status = 'error'
    GROUP BY connection_id
)
UPDATE bank_connections bc
SET consecutive_failures = COALESCE(s.failures, 0)
FROM streak s
WHERE bc.id = s.connection_id;

-- Backfill last_error_at from most recent error sync log.
UPDATE bank_connections bc
SET last_error_at = sub.last_err
FROM (
    SELECT connection_id, MAX(started_at) AS last_err
    FROM sync_logs
    WHERE status = 'error'
    GROUP BY connection_id
) sub
WHERE bc.id = sub.connection_id;

-- +goose Down
ALTER TABLE bank_connections DROP COLUMN IF EXISTS last_error_at;
ALTER TABLE bank_connections DROP COLUMN IF EXISTS consecutive_failures;
