-- +goose Up
-- Register the encryption_key_fingerprint slot in app_config.
--
-- The actual fingerprint value is written at server startup via
-- trust-on-first-use (TOFU) — first boot with a key records its fingerprint,
-- every subsequent boot compares against the stored value so a changed key
-- is a visible mismatch (breadbox doctor in #687). Storing it in app_config
-- keeps the write surface aligned with every other server-managed setting
-- and avoids a dedicated table for a single KV row.
--
-- The insert is NULL-valued + ON CONFLICT DO NOTHING so this migration is
-- safe to replay and does not clobber a fingerprint already written by a
-- running server.
INSERT INTO app_config (key, value)
VALUES ('encryption_key_fingerprint', NULL)
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM app_config WHERE key = 'encryption_key_fingerprint';
