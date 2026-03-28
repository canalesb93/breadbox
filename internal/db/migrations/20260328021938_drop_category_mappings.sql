-- +goose Up
DROP TABLE IF EXISTS category_mappings;

-- +goose Down
CREATE TABLE category_mappings (
    id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    provider          TEXT          NOT NULL,
    provider_category TEXT          NOT NULL,
    category_id       UUID          NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_category)
);

CREATE INDEX idx_category_mappings_provider ON category_mappings(provider);
CREATE INDEX idx_category_mappings_category_id ON category_mappings(category_id);
