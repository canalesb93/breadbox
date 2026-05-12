-- +goose Up
-- Extend the annotations.kind CHECK constraint to recognise transaction
-- lifecycle events on the activity timeline:
--   - transaction_deleted: written when a transaction is soft-deleted via the
--     REST API (DELETE /transactions/{id}).
--   - transaction_restored: written when a soft-deleted transaction is
--     restored via POST /transactions/{id}/restore.
--
-- Drop + add must run in a single SQL transaction so the table is never
-- without a constraint. Goose wraps each migration in a transaction by
-- default, which gives us the atomicity we need; we just drop and re-add
-- inline.
--
-- Additive in the shared-DB sense — older `breadbox serve` processes never
-- write the new kinds, so they keep working even before they pick up the
-- code that emits them.
ALTER TABLE annotations DROP CONSTRAINT IF EXISTS annotations_kind_check;
ALTER TABLE annotations ADD CONSTRAINT annotations_kind_check
  CHECK (kind IN (
    'comment',
    'rule_applied',
    'tag_added',
    'tag_removed',
    'category_set',
    'sync_started',
    'sync_updated',
    'transaction_deleted',
    'transaction_restored'
  ));

-- +goose Down
ALTER TABLE annotations DROP CONSTRAINT IF EXISTS annotations_kind_check;
ALTER TABLE annotations ADD CONSTRAINT annotations_kind_check
  CHECK (kind IN (
    'comment',
    'rule_applied',
    'tag_added',
    'tag_removed',
    'category_set',
    'sync_started',
    'sync_updated'
  ));
