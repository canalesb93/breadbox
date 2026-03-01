-- +goose Up
CREATE TABLE admin_accounts (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username        TEXT        NOT NULL UNIQUE,
    hashed_password BYTEA       NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX admin_accounts_username_idx ON admin_accounts (username);

-- +goose Down
DROP INDEX IF EXISTS admin_accounts_username_idx;
DROP TABLE IF EXISTS admin_accounts;
