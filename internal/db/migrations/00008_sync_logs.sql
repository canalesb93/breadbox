-- +goose Up
CREATE TABLE sync_logs (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    connection_id  UUID         NOT NULL REFERENCES bank_connections (id) ON DELETE CASCADE,
    "trigger"      sync_trigger NOT NULL,
    added_count    INTEGER      NOT NULL DEFAULT 0,
    modified_count INTEGER      NOT NULL DEFAULT 0,
    removed_count  INTEGER      NOT NULL DEFAULT 0,
    status         sync_status  NOT NULL DEFAULT 'in_progress',
    error_message  TEXT         NULL,
    started_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    completed_at   TIMESTAMPTZ  NULL
);

CREATE INDEX sync_logs_connection_id_started_at_idx ON sync_logs (connection_id, started_at DESC);
CREATE INDEX sync_logs_started_at_idx ON sync_logs (started_at DESC);

-- +goose Down
DROP INDEX IF EXISTS sync_logs_started_at_idx;
DROP INDEX IF EXISTS sync_logs_connection_id_started_at_idx;
DROP TABLE IF EXISTS sync_logs;
