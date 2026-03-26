-- +goose Up
CREATE TABLE agent_reports (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    title             TEXT        NOT NULL,
    body              TEXT        NOT NULL,
    created_by_type   TEXT        NOT NULL DEFAULT 'agent',
    created_by_id     TEXT        NULL,
    created_by_name   TEXT        NOT NULL DEFAULT 'Unknown',
    read_at           TIMESTAMPTZ NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX agent_reports_unread_idx ON agent_reports (created_at DESC) WHERE read_at IS NULL;
CREATE INDEX agent_reports_created_at_idx ON agent_reports (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS agent_reports;
