-- +goose Up
-- Global connector library: custom MCP servers (Phase 1, HTTP transport)
-- configured ONCE with their credential, then enabled per-workflow. This
-- replaces the per-workflow secret-bearing config — workflows.connectors now
-- holds just a JSON array of enabled connector names (strings) referencing
-- rows here. The header secret is AES-256-GCM-encrypted at rest (same
-- ENCRYPTION_KEY as provider tokens); plaintext never lands in this table.
CREATE TABLE connector_library (
    id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id          TEXT         NOT NULL DEFAULT '',
    name              TEXT         NOT NULL UNIQUE,
    url               TEXT         NOT NULL,
    header_name       TEXT         NULL,
    secret_ciphertext TEXT         NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_connector_library_short_id
    BEFORE INSERT ON connector_library
    FOR EACH ROW EXECUTE FUNCTION set_short_id();

CREATE UNIQUE INDEX connector_library_short_id_idx ON connector_library (short_id);

-- +goose Down
DROP TABLE IF EXISTS connector_library;
