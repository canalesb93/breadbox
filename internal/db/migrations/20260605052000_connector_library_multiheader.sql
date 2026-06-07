-- +goose Up
-- Connectors gain multiple custom headers, a transport type, and an optional
-- note (operator guidance injected into the agent's prompt). Header VALUES are
-- secret (auth tokens) so they're stored AES-256-GCM-encrypted as one JSON map;
-- header NAMES are not secret and stay cleartext so the directory + edit form
-- can list them without decrypting. Replaces the single header_name/secret pair.
-- The connector_library table is new in this same unreleased PR, so dropping the
-- old single-header columns is safe — nothing else references them.
ALTER TABLE connector_library DROP COLUMN IF EXISTS header_name;
ALTER TABLE connector_library DROP COLUMN IF EXISTS secret_ciphertext;

ALTER TABLE connector_library ADD COLUMN transport TEXT NOT NULL DEFAULT 'http';
ALTER TABLE connector_library ADD COLUMN note TEXT;
ALTER TABLE connector_library ADD COLUMN header_names JSONB NOT NULL DEFAULT '[]';
ALTER TABLE connector_library ADD COLUMN header_values_ciphertext TEXT;

-- +goose Down
ALTER TABLE connector_library DROP COLUMN IF EXISTS transport;
ALTER TABLE connector_library DROP COLUMN IF EXISTS note;
ALTER TABLE connector_library DROP COLUMN IF EXISTS header_names;
ALTER TABLE connector_library DROP COLUMN IF EXISTS header_values_ciphertext;
ALTER TABLE connector_library ADD COLUMN header_name TEXT;
ALTER TABLE connector_library ADD COLUMN secret_ciphertext TEXT;
