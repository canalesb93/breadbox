-- +goose Up
DROP TABLE IF EXISTS audit_log;

-- +goose Down
-- Audit log table was removed intentionally; no restore needed.
