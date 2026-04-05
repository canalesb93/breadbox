-- +goose Up
-- Remove the confidence threshold config (no longer used).
DELETE FROM app_config WHERE key = 'review_confidence_threshold';

-- Change default for review_auto_enqueue to false for new installations.
-- Existing installations keep their current setting.
INSERT INTO app_config (key, value) VALUES ('review_auto_enqueue', 'false')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
INSERT INTO app_config (key, value) VALUES ('review_confidence_threshold', '0.5')
ON CONFLICT (key) DO NOTHING;
