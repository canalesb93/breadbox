-- +goose Up
-- Soft-delete tombstones for comment annotations. Hard-deleting wiped any
-- record that a comment ever existed; the activity timeline lost its audit
-- trail. With deleted_at set, ListAnnotations keeps the row and the UI
-- renders a muted single-line "<Actor> deleted a comment" tombstone.
ALTER TABLE annotations ADD COLUMN deleted_at TIMESTAMPTZ NULL;
CREATE INDEX IF NOT EXISTS annotations_deleted_at_idx ON annotations (deleted_at);

-- +goose Down
DROP INDEX IF EXISTS annotations_deleted_at_idx;
ALTER TABLE annotations DROP COLUMN IF EXISTS deleted_at;
