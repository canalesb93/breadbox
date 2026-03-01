-- +goose Up
INSERT INTO app_config (key, value) VALUES
    ('plaid_client_id', ''),
    ('plaid_secret', ''),
    ('plaid_env', 'sandbox'),
    ('webhook_url', ''),
    ('sync_interval_hours', '12'),
    ('setup_complete', 'false')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM app_config WHERE key IN (
    'plaid_client_id',
    'plaid_secret',
    'plaid_env',
    'webhook_url',
    'sync_interval_hours',
    'setup_complete'
);
