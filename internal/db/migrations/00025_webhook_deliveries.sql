-- +goose Up
CREATE TABLE webhook_deliveries (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    event           TEXT          NOT NULL,
    url             TEXT          NOT NULL,
    payload         JSONB         NOT NULL,
    delivery_id     UUID          NOT NULL UNIQUE,
    status          TEXT          NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'success', 'failed')),
    attempts        INTEGER       NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ   NULL,
    next_retry_at   TIMESTAMPTZ   NULL,
    response_status INTEGER       NULL,
    response_body   TEXT          NULL,
    error_message   TEXT          NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX webhook_deliveries_status_idx ON webhook_deliveries (status, next_retry_at)
    WHERE status = 'pending';
CREATE INDEX webhook_deliveries_created_at_idx ON webhook_deliveries (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS webhook_deliveries;
