-- +goose Up
-- transactions.metadata: free-form JSONB key/value store for enrichment data that
-- isn't a first-class column. Users define the keys; agents (and users) populate
-- them via the scoped metadata ops. NOT NULL DEFAULT '{}' so callers can always
-- read metadata->>'key' and merge without a NULL guard; "clear" writes '{}', not NULL.
ALTER TABLE transactions ADD COLUMN metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE transactions DROP COLUMN IF EXISTS metadata;
