-- +goose Up

-- Phase 1 of the review-system redesign: codify rule actions, drop the denormalized
-- transaction_rules.category_id column, allow NULL conditions to mean match-all,
-- and add an explicit `trigger` column so rules know whether to fire on insert vs update.
--
-- The existing actions JSONB shape is [{"field": "category", "value": "<slug>"}].
-- Backfill rewrites every action into the typed shape:
--   [{"type": "set_category", "category_slug": "<slug>"}]
-- Unknown action shapes are passed through untouched (read-time tolerance).

-- +goose StatementBegin
UPDATE transaction_rules
SET actions = COALESCE(
    (
        SELECT jsonb_agg(
            CASE
                WHEN elem->>'field' = 'category' AND elem->>'value' IS NOT NULL THEN
                    jsonb_build_object('type', 'set_category', 'category_slug', elem->>'value')
                ELSE elem
            END
        )
        FROM jsonb_array_elements(actions) elem
    ),
    '[]'::jsonb
)
WHERE jsonb_typeof(actions) = 'array';
-- +goose StatementEnd

-- Drop the denormalized category_id column. Rules now express category-setting
-- via actions[{type:"set_category", category_slug:"..."}] only.
DROP INDEX IF EXISTS transaction_rules_category_id_idx;
ALTER TABLE transaction_rules DROP COLUMN category_id;

-- Allow NULL conditions to mean "match every transaction". Used by the seeded
-- needs-review rule (and any catch-all rule a user wants).
ALTER TABLE transaction_rules ALTER COLUMN conditions DROP NOT NULL;

-- Add explicit trigger column so rules can target inserts (sync's new-transaction path),
-- updates (changed transactions), or both.
ALTER TABLE transaction_rules
    ADD COLUMN trigger TEXT NOT NULL DEFAULT 'on_create'
        CHECK (trigger IN ('on_create', 'on_update', 'always'));

-- The review_auto_enqueue config flag is removed entirely. Phase 2 will re-enable
-- review enqueueing via a seeded rule + tag; until then sync no longer auto-enqueues.
DELETE FROM app_config WHERE key = 'review_auto_enqueue';

-- +goose Down

-- Re-add category_id and backfill from the typed actions array.
ALTER TABLE transaction_rules ADD COLUMN category_id UUID NULL REFERENCES categories(id) ON DELETE CASCADE;
CREATE INDEX transaction_rules_category_id_idx ON transaction_rules(category_id);

-- +goose StatementBegin
UPDATE transaction_rules tr
SET category_id = c.id
FROM categories c
WHERE c.slug = (
    SELECT elem->>'category_slug'
    FROM jsonb_array_elements(tr.actions) elem
    WHERE elem->>'type' = 'set_category'
    LIMIT 1
);
-- +goose StatementEnd

-- Revert actions JSONB to the legacy [{field, value}] shape.
-- +goose StatementBegin
UPDATE transaction_rules
SET actions = COALESCE(
    (
        SELECT jsonb_agg(
            CASE
                WHEN elem->>'type' = 'set_category' AND elem->>'category_slug' IS NOT NULL THEN
                    jsonb_build_object('field', 'category', 'value', elem->>'category_slug')
                ELSE elem
            END
        )
        FROM jsonb_array_elements(actions) elem
    ),
    '[]'::jsonb
)
WHERE jsonb_typeof(actions) = 'array';
-- +goose StatementEnd

ALTER TABLE transaction_rules DROP COLUMN trigger;
ALTER TABLE transaction_rules ALTER COLUMN conditions SET NOT NULL;

INSERT INTO app_config (key, value) VALUES ('review_auto_enqueue', 'false')
ON CONFLICT (key) DO NOTHING;
