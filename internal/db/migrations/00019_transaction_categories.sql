-- +goose Up
ALTER TABLE transactions
    ADD COLUMN category_id UUID NULL REFERENCES categories(id) ON DELETE SET NULL,
    ADD COLUMN category_override BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX transactions_category_id_idx ON transactions(category_id);

-- +goose Down
DROP INDEX IF EXISTS transactions_category_id_idx;
ALTER TABLE transactions
    DROP COLUMN IF EXISTS category_override,
    DROP COLUMN IF EXISTS category_id;
