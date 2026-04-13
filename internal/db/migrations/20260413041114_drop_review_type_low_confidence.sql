-- +goose Up
-- Remove vestigial 'low_confidence' from review_type. No code path ever
-- sets it (only 'uncategorized' or 'new_transaction'), so it was dead weight
-- on the CHECK constraint. Defensively remap any stragglers to 'manual'.
UPDATE review_queue SET review_type = 'manual' WHERE review_type = 'low_confidence';
ALTER TABLE review_queue DROP CONSTRAINT IF EXISTS review_queue_review_type_check;
ALTER TABLE review_queue ADD CONSTRAINT review_queue_review_type_check
    CHECK (review_type IN ('new_transaction', 'uncategorized', 'manual', 're_review'));

-- +goose Down
ALTER TABLE review_queue DROP CONSTRAINT IF EXISTS review_queue_review_type_check;
ALTER TABLE review_queue ADD CONSTRAINT review_queue_review_type_check
    CHECK (review_type IN ('new_transaction', 'uncategorized', 'low_confidence', 'manual', 're_review'));
