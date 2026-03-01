-- +goose Up
CREATE TABLE app_config (
    key        VARCHAR(255) PRIMARY KEY,
    value      TEXT         NULL,
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS app_config;
