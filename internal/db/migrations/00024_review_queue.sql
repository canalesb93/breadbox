-- +goose Up
CREATE TABLE review_queue (
    id                    UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id        UUID          NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
    review_type           TEXT          NOT NULL CHECK (review_type IN ('new_transaction', 'uncategorized', 'low_confidence', 'manual')),
    status                TEXT          NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'skipped')),
    suggested_category_id UUID          NULL REFERENCES categories (id) ON DELETE SET NULL,
    confidence_score      NUMERIC(5,4) NULL,
    reviewer_type         TEXT          NULL CHECK (reviewer_type IN ('user', 'agent')),
    reviewer_id           TEXT          NULL,
    reviewer_name         TEXT          NULL,
    review_note           TEXT          NULL,
    resolved_category_id  UUID          NULL REFERENCES categories (id) ON DELETE SET NULL,
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    reviewed_at           TIMESTAMPTZ   NULL,

    CONSTRAINT review_queue_reviewer_complete CHECK (
        (status = 'pending' AND reviewer_type IS NULL AND reviewer_id IS NULL AND reviewed_at IS NULL)
        OR (status != 'pending' AND reviewer_type IS NOT NULL AND reviewed_at IS NOT NULL)
    )
);

CREATE INDEX review_queue_status_idx ON review_queue (status, created_at ASC) WHERE status = 'pending';
CREATE INDEX review_queue_transaction_id_idx ON review_queue (transaction_id);
CREATE INDEX review_queue_reviewed_at_idx ON review_queue (reviewed_at DESC) WHERE status != 'pending';

-- Prevent duplicate pending reviews for the same transaction.
CREATE UNIQUE INDEX review_queue_pending_unique_idx ON review_queue (transaction_id) WHERE status = 'pending';

-- Seed review config values
INSERT INTO app_config (key, value) VALUES
    ('review_auto_enqueue', 'true'),
    ('review_confidence_threshold', '0.5')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DROP INDEX IF EXISTS review_queue_pending_unique_idx;
DROP INDEX IF EXISTS review_queue_reviewed_at_idx;
DROP INDEX IF EXISTS review_queue_transaction_id_idx;
DROP INDEX IF EXISTS review_queue_status_idx;
DROP TABLE IF EXISTS review_queue;

DELETE FROM app_config WHERE key IN ('review_auto_enqueue', 'review_confidence_threshold');
