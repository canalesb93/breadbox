-- +goose Up

CREATE TABLE webhook_events (
    id               UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    provider         provider_type NOT NULL,
    event_type       TEXT          NOT NULL,
    connection_id    UUID          NULL REFERENCES bank_connections(id) ON DELETE SET NULL,
    raw_payload_hash TEXT          NOT NULL,
    status           TEXT          NOT NULL DEFAULT 'received',
    error_message    TEXT          NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX webhook_events_created_at_idx ON webhook_events(created_at DESC);
CREATE INDEX webhook_events_provider_idx ON webhook_events(provider);
CREATE INDEX webhook_events_connection_id_idx ON webhook_events(connection_id) WHERE connection_id IS NOT NULL;
CREATE INDEX webhook_events_status_idx ON webhook_events(status);

-- +goose Down

DROP TABLE IF EXISTS webhook_events;
