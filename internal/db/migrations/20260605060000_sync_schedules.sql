-- +goose Up

-- sync_schedules: first-class, wall-clock-anchored sync schedules. Replaces the
-- single global `sync_interval_minutes` interval (relative to last_synced_at,
-- which drifted) with cron specs evaluated against the wall clock.
--
-- A schedule targets connections via the join table below, or every connection
-- (incl. future ones) via applies_to_all. A connection's effective schedules are
-- the UNION of all enabled schedules that apply to it; it is due when ANY of them
-- has fired since its last successful sync. Many schedules per connection and many
-- connections per schedule are both first-class — the one-vs-many distinction is
-- purely how many rows exist, never a schema change.
--
-- Additive-only: two new tables + one app_config-derived seed row. Safe to apply
-- to the shared dev DB alongside running servers.
CREATE TABLE sync_schedules (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id       TEXT        NOT NULL UNIQUE,
    name           TEXT        NOT NULL,
    cron           TEXT        NOT NULL,
    preset         TEXT        NULL,
    applies_to_all BOOLEAN     NOT NULL DEFAULT FALSE,
    enabled        BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_short_id_sync_schedules
    BEFORE INSERT ON sync_schedules
    FOR EACH ROW EXECUTE FUNCTION set_short_id();

-- The many-to-many mapping. CASCADE on both sides: deleting a schedule or a
-- connection drops the mapping rows (the mapping is derived, never the source of
-- truth for either entity).
CREATE TABLE sync_schedule_connections (
    schedule_id   UUID NOT NULL REFERENCES sync_schedules(id) ON DELETE CASCADE,
    connection_id UUID NOT NULL REFERENCES bank_connections(id) ON DELETE CASCADE,
    PRIMARY KEY (schedule_id, connection_id)
);

-- Reverse lookup: "which schedules target this connection" during evaluation.
CREATE INDEX sync_schedule_connections_conn_idx
    ON sync_schedule_connections (connection_id);

-- Seed one applies_to_all schedule from the existing global interval so every
-- connection keeps an equivalent wall-clock cadence on the first tick after
-- migrating. The old `sync_interval_minutes` app_config key stays readable for
-- one release as a fallback (see internal/sync/scheduler.go) but is no longer the
-- source of truth.
INSERT INTO sync_schedules (name, cron, preset, applies_to_all, enabled)
SELECT
    'Default schedule',
    CASE COALESCE((SELECT value FROM app_config WHERE key = 'sync_interval_minutes'), '720')
        WHEN '15'   THEN '*/15 * * * *'
        WHEN '30'   THEN '*/30 * * * *'
        WHEN '60'   THEN '0 * * * *'
        WHEN '240'  THEN '0 */4 * * *'
        WHEN '480'  THEN '0 */8 * * *'
        WHEN '720'  THEN '0 */12 * * *'
        WHEN '1440' THEN '0 0 * * *'
        ELSE             '0 */12 * * *'
    END,
    'interval-migrated',
    TRUE,
    TRUE;

-- +goose Down
DROP TABLE IF EXISTS sync_schedule_connections;
DROP TABLE IF EXISTS sync_schedules;
