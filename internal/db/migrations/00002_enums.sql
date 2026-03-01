-- +goose Up
CREATE TYPE provider_type AS ENUM ('plaid', 'teller', 'csv');

CREATE TYPE connection_status AS ENUM ('active', 'error', 'pending_reauth', 'disconnected');

CREATE TYPE sync_trigger AS ENUM ('cron', 'webhook', 'manual', 'initial');

CREATE TYPE sync_status AS ENUM ('in_progress', 'success', 'error');

-- +goose Down
DROP TYPE IF EXISTS sync_status;
DROP TYPE IF EXISTS sync_trigger;
DROP TYPE IF EXISTS connection_status;
DROP TYPE IF EXISTS provider_type;
