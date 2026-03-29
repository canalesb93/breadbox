-- +goose Up
ALTER TABLE transaction_rules ADD COLUMN actions JSONB NOT NULL DEFAULT '[]';

-- Backfill existing rules: convert category_id into actions array
UPDATE transaction_rules tr
SET actions = jsonb_build_array(
    jsonb_build_object('field', 'category', 'value', c.slug)
)
FROM categories c
WHERE tr.category_id = c.id
  AND tr.category_id IS NOT NULL;

-- +goose Down
ALTER TABLE transaction_rules DROP COLUMN IF EXISTS actions;
