-- +goose Up
-- Per-workflow custom MCP connectors (Phase 1: HTTP transport, bring-your-own
-- credential). A JSON array of objects:
--   {"name":"gmail","url":"https://…/mcp","header_name":"Authorization",
--    "secret_ciphertext":"<hex AES-256-GCM>"}
-- The secret is stored encrypted (same ENCRYPTION_KEY as provider tokens);
-- plaintext never lands in this column. Empty array = no extra connectors.
-- Additive with a DEFAULT, so sibling servers on the shared dev DB keep working.
ALTER TABLE workflows ADD COLUMN connectors JSONB NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE workflows DROP COLUMN connectors;
