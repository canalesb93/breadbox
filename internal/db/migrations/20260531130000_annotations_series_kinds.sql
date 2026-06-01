-- +goose Up
-- Extend the annotations.kind CHECK constraint to recognise recurring-series
-- membership events on the activity timeline:
--   - series_assigned: written when a transaction is linked to a recurring
--     series (manual assign, detection auto-link, or manual create).
--   - series_unlinked: written when a transaction is detached from a series.
--
-- Drop + add run in goose's per-migration transaction so the table is never
-- without a constraint. Additive in the shared-DB sense: the new constraint
-- only WIDENS the allowed set, so older `breadbox serve` processes (which never
-- write the new kinds) keep working before they pick up the emitting code.
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
    'transaction_restored',
    'series_assigned',
    'series_unlinked'
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
    'sync_updated',
    'transaction_deleted',
    'transaction_restored'
  ));
