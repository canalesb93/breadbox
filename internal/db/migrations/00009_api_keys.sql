-- +goose Up
CREATE TABLE api_keys (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    key_hash    TEXT        NOT NULL UNIQUE,
    key_prefix  TEXT        NOT NULL,
    last_used_at TIMESTAMPTZ NULL,
    revoked_at  TIMESTAMPTZ NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX api_keys_key_hash_idx ON api_keys (key_hash);

-- +goose Down
DROP INDEX IF EXISTS api_keys_key_hash_idx;
DROP TABLE IF EXISTS api_keys;
