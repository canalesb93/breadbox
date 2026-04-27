-- +goose Up
-- Extend the annotations.kind CHECK constraint to recognise sync events on the
-- activity timeline:
--   - sync_started: written when a transaction is first imported (initial sync).
--   - sync_updated: written when a subsequent sync flipped pending status.
--
-- Drop + add must run in a single SQL transaction so the table is never without
-- a constraint. Goose wraps each migration in a transaction by default, which
-- gives us the atomicity we need; we just drop and re-add inline.
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

-- +goose Down
ALTER TABLE annotations DROP CONSTRAINT IF EXISTS annotations_kind_check;
ALTER TABLE annotations ADD CONSTRAINT annotations_kind_check
  CHECK (kind IN (
    'comment',
    'rule_applied',
    'tag_added',
    'tag_removed',
    'category_set'
  ));
