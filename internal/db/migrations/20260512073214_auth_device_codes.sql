-- +goose Up
-- Device-code flow for `breadbox auth login` on a remote host. A CLI
-- creates a row (status='pending') and polls. A signed-in admin opens
-- the verification URL, mints an API key, and we copy the plaintext into
-- api_key_secret so the next poll returns it once.
CREATE TABLE auth_device_codes (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_code    TEXT NOT NULL UNIQUE,
    user_code      TEXT NOT NULL UNIQUE,
    status         TEXT NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'approved', 'denied', 'expired')),
    api_key_id     UUID REFERENCES api_keys(id) ON DELETE SET NULL,
    api_key_secret TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at     TIMESTAMPTZ NOT NULL,
    approved_at    TIMESTAMPTZ,
    approved_by    UUID REFERENCES auth_accounts(id) ON DELETE SET NULL
);

CREATE INDEX idx_auth_device_codes_pending
    ON auth_device_codes (status)
    WHERE status = 'pending';

CREATE INDEX idx_auth_device_codes_user_code
    ON auth_device_codes (user_code);

-- +goose Down
DROP TABLE auth_device_codes;
