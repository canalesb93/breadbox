-- +goose Up

-- category_id: the Breadbox category a user assigns to a staged row (via the
-- preview's bulk "set category" or inline edit). NULL = let transaction rules
-- categorize at/after import, matching the legacy CSV behavior. Distinct from
-- parsed_category, which is the raw provider category string from the file.
ALTER TABLE csv_import_rows
    ADD COLUMN IF NOT EXISTS category_id UUID NULL REFERENCES categories (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE csv_import_rows DROP COLUMN IF EXISTS category_id;
