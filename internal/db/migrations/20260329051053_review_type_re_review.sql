-- +goose Up
ALTER TABLE review_queue DROP CONSTRAINT IF EXISTS review_queue_review_type_check;
ALTER TABLE review_queue ADD CONSTRAINT review_queue_review_type_check
    CHECK (review_type IN ('new_transaction', 'uncategorized', 'low_confidence', 'manual', 're_review'));

-- +goose Down
ALTER TABLE review_queue DROP CONSTRAINT IF EXISTS review_queue_review_type_check;
ALTER TABLE review_queue ADD CONSTRAINT review_queue_review_type_check
    CHECK (review_type IN ('new_transaction', 'uncategorized', 'low_confidence', 'manual'));
