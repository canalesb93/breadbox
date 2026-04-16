-- +goose Up
-- +goose StatementBegin
ALTER TABLE transaction_comments
    ADD COLUMN review_id UUID NULL REFERENCES review_queue (id) ON DELETE SET NULL;

-- Partial UNIQUE: at most one linked note-comment per review. Guards against
-- retries or duplicate writes producing comments that would be silently
-- overwritten in the activity-timeline lookup (commentsByReview map).
CREATE UNIQUE INDEX transaction_comments_review_id_idx
    ON transaction_comments (review_id) WHERE review_id IS NOT NULL;

-- Backfill: one comment per resolved review that has a non-empty note.
-- Attribution mirrors the reviewer fields; timestamp uses reviewed_at so
-- the comment aligns with the resolution event in the activity timeline.
INSERT INTO transaction_comments
    (transaction_id, author_type, author_id, author_name, content, review_id, created_at, updated_at)
SELECT
    rq.transaction_id,
    COALESCE(rq.reviewer_type, 'system'),
    rq.reviewer_id,
    COALESCE(rq.reviewer_name, 'System'),
    rq.review_note,
    rq.id,
    COALESCE(rq.reviewed_at, rq.created_at),
    COALESCE(rq.reviewed_at, rq.created_at)
FROM review_queue rq
WHERE rq.review_note IS NOT NULL
  AND rq.review_note <> ''
  AND NOT EXISTS (
      SELECT 1 FROM transaction_comments tc WHERE tc.review_id = rq.id
  );

ALTER TABLE review_queue DROP COLUMN review_note;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE review_queue ADD COLUMN review_note TEXT NULL;

UPDATE review_queue rq
   SET review_note = tc.content
  FROM transaction_comments tc
 WHERE tc.review_id = rq.id;

DROP INDEX IF EXISTS transaction_comments_review_id_idx;
ALTER TABLE transaction_comments DROP COLUMN review_id;
-- +goose StatementEnd
