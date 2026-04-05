-- +goose Up

-- Consolidated auth table: replaces admin_accounts and member_accounts.
-- Named "auth_accounts" to avoid collision with bank "accounts" table.
CREATE TABLE auth_accounts (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID        REFERENCES users(id) ON DELETE CASCADE,  -- NULL for the initial admin
    username        TEXT        NOT NULL UNIQUE,
    hashed_password BYTEA       NULL,  -- NULL until member sets their password via setup flow
    role            TEXT        NOT NULL DEFAULT 'viewer' CHECK (role IN ('admin', 'editor', 'viewer')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX auth_accounts_username_idx ON auth_accounts (username);
CREATE UNIQUE INDEX auth_accounts_user_id_idx ON auth_accounts (user_id) WHERE user_id IS NOT NULL;

-- Migrate admin_accounts → auth_accounts (role='admin', user_id=NULL, password always set).
INSERT INTO auth_accounts (id, user_id, username, hashed_password, role, created_at, updated_at)
SELECT id, NULL, username, hashed_password, 'admin', created_at, created_at
FROM admin_accounts;

-- Migrate member_accounts → auth_accounts (preserve role mapping: 'admin'→'admin', 'member'→'viewer').
INSERT INTO auth_accounts (id, user_id, username, hashed_password, role, created_at, updated_at)
SELECT id, user_id, username, hashed_password,
       CASE WHEN role = 'admin' THEN 'admin' ELSE 'viewer' END,
       created_at, updated_at
FROM member_accounts;

-- Drop old tables.
DROP TABLE IF EXISTS member_accounts;
DROP TABLE IF EXISTS admin_accounts;

-- +goose Down

-- Recreate admin_accounts.
CREATE TABLE admin_accounts (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username        TEXT        NOT NULL UNIQUE,
    hashed_password BYTEA       NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX admin_accounts_username_idx ON admin_accounts (username);

-- Recreate member_accounts.
CREATE TABLE member_accounts (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    username        TEXT        NOT NULL UNIQUE,
    hashed_password BYTEA       NULL,
    role            TEXT        NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX member_accounts_username_idx ON member_accounts (username);
CREATE UNIQUE INDEX member_accounts_user_id_idx ON member_accounts (user_id);

-- Migrate back: admin rows (user_id IS NULL) → admin_accounts.
INSERT INTO admin_accounts (id, username, hashed_password, created_at)
SELECT id, username, hashed_password, created_at
FROM auth_accounts WHERE user_id IS NULL;

-- Migrate back: non-admin rows → member_accounts.
INSERT INTO member_accounts (id, user_id, username, hashed_password, role, created_at, updated_at)
SELECT id, user_id, username, hashed_password,
       CASE WHEN role = 'admin' THEN 'admin' ELSE 'member' END,
       created_at, updated_at
FROM auth_accounts WHERE user_id IS NOT NULL;

DROP TABLE IF EXISTS auth_accounts;
