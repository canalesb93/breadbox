-- +goose Up
CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BYTEA NOT NULL,
    expiry TIMESTAMPTZ NOT NULL
);

CREATE INDEX sessions_expiry_idx ON sessions (expiry);

-- +goose Down
DROP INDEX IF EXISTS sessions_expiry_idx;
DROP TABLE IF EXISTS sessions;
