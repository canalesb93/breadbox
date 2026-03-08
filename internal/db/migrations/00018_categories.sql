-- +goose Up
CREATE TABLE categories (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    parent_id   UUID NULL REFERENCES categories(id) ON DELETE CASCADE,
    icon        TEXT NULL,
    color       TEXT NULL,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    is_system   BOOLEAN NOT NULL DEFAULT FALSE,
    hidden      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX categories_parent_id_idx ON categories(parent_id);
CREATE INDEX categories_slug_idx ON categories(slug);

CREATE TABLE category_mappings (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider          provider_type NOT NULL,
    provider_category TEXT NOT NULL,
    category_id       UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider, provider_category)
);

CREATE INDEX category_mappings_provider_idx ON category_mappings(provider);
CREATE INDEX category_mappings_category_id_idx ON category_mappings(category_id);

-- +goose Down
DROP TABLE IF EXISTS category_mappings;
DROP TABLE IF EXISTS categories;
