-- +goose Up

-- Member accounts: household members who can log in with scoped access.
-- Linked to the users (family members) table. Role determines permissions.
CREATE TABLE member_accounts (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    username        TEXT        NOT NULL UNIQUE,
    hashed_password BYTEA       NULL, -- NULL until member sets their password
    role            TEXT        NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX member_accounts_username_idx ON member_accounts (username);
CREATE UNIQUE INDEX member_accounts_user_id_idx ON member_accounts (user_id);

-- +goose Down
DROP INDEX IF EXISTS member_accounts_user_id_idx;
DROP INDEX IF EXISTS member_accounts_username_idx;
DROP TABLE IF EXISTS member_accounts;
