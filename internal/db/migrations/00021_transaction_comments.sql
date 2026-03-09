-- +goose Up
CREATE TABLE transaction_comments (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id  UUID          NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
    author_type     TEXT          NOT NULL CHECK (author_type IN ('user', 'agent', 'system')),
    author_id       TEXT          NULL,
    author_name     TEXT          NOT NULL,
    content         TEXT          NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX transaction_comments_transaction_id_idx ON transaction_comments (transaction_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS transaction_comments_transaction_id_idx;
DROP TABLE IF EXISTS transaction_comments;
