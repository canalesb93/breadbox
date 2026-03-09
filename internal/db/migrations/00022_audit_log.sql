-- +goose Up
CREATE TABLE audit_log (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type     TEXT          NOT NULL,
    entity_id       UUID          NOT NULL,
    action          TEXT          NOT NULL CHECK (action IN ('create', 'update', 'delete')),
    field           TEXT          NULL,
    old_value       TEXT          NULL,
    new_value       TEXT          NULL,
    actor_type      TEXT          NOT NULL CHECK (actor_type IN ('user', 'agent', 'system')),
    actor_id        TEXT          NULL,
    actor_name      TEXT          NOT NULL,
    metadata        JSONB         NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX audit_log_entity_idx ON audit_log (entity_type, entity_id, created_at DESC);
CREATE INDEX audit_log_created_at_idx ON audit_log (created_at DESC);
CREATE INDEX audit_log_actor_idx ON audit_log (actor_type, actor_id);

-- +goose Down
DROP INDEX IF EXISTS audit_log_actor_idx;
DROP INDEX IF EXISTS audit_log_created_at_idx;
DROP INDEX IF EXISTS audit_log_entity_idx;
DROP TABLE IF EXISTS audit_log;
