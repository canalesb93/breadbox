-- +goose Up

-- OAuth clients: managed like API keys, used by external services (e.g., Claude connectors)
-- to authenticate with the MCP server via OAuth 2.1.
CREATE TABLE oauth_clients (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT          NOT NULL,
    client_id           TEXT          NOT NULL UNIQUE,
    client_secret_hash  TEXT          NOT NULL,
    client_id_prefix    TEXT          NOT NULL,
    redirect_uris       TEXT[]        NOT NULL DEFAULT '{}',
    scope               TEXT          NOT NULL DEFAULT 'full_access',
    revoked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

-- Authorization codes: short-lived, for OAuth 2.1 authorization code flow with PKCE.
CREATE TABLE oauth_authorization_codes (
    code_hash              TEXT          PRIMARY KEY,
    client_id              TEXT          NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
    admin_id               UUID          NOT NULL REFERENCES admin_accounts(id) ON DELETE CASCADE,
    redirect_uri           TEXT          NOT NULL,
    scope                  TEXT          NOT NULL,
    code_challenge         TEXT          NOT NULL,
    code_challenge_method  TEXT          NOT NULL DEFAULT 'S256',
    expires_at             TIMESTAMPTZ   NOT NULL,
    used_at                TIMESTAMPTZ,
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX oauth_auth_codes_client_idx ON oauth_authorization_codes(client_id);
CREATE INDEX oauth_auth_codes_expires_idx ON oauth_authorization_codes(expires_at);

-- Access tokens: bearer tokens for MCP/API access.
CREATE TABLE oauth_access_tokens (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash      TEXT          NOT NULL UNIQUE,
    client_id       TEXT          NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
    admin_id        UUID          NOT NULL REFERENCES admin_accounts(id) ON DELETE CASCADE,
    scope           TEXT          NOT NULL DEFAULT 'full_access',
    last_used_at    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ   NOT NULL,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX oauth_access_tokens_hash_idx ON oauth_access_tokens(token_hash);
CREATE INDEX oauth_access_tokens_client_idx ON oauth_access_tokens(client_id);

-- Refresh tokens: long-lived, used to obtain new access tokens.
CREATE TABLE oauth_refresh_tokens (
    id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash        TEXT          NOT NULL UNIQUE,
    access_token_id   UUID          NOT NULL REFERENCES oauth_access_tokens(id) ON DELETE CASCADE,
    expires_at        TIMESTAMPTZ   NOT NULL,
    revoked_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX oauth_refresh_tokens_hash_idx ON oauth_refresh_tokens(token_hash);
CREATE INDEX oauth_refresh_tokens_access_idx ON oauth_refresh_tokens(access_token_id);

-- +goose Down
DROP TABLE IF EXISTS oauth_refresh_tokens;
DROP TABLE IF EXISTS oauth_access_tokens;
DROP TABLE IF EXISTS oauth_authorization_codes;
DROP TABLE IF EXISTS oauth_clients;
