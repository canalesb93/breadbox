-- name: EnqueueReview :one
INSERT INTO review_queue (transaction_id, review_type, suggested_category_id, confidence_score)
VALUES ($1, $2, $3, $4)
ON CONFLICT (transaction_id) WHERE status = 'pending' DO NOTHING
RETURNING *;

-- name: GetReviewByID :one
SELECT * FROM review_queue WHERE id = $1;

-- name: GetPendingReviewByTransactionID :one
SELECT * FROM review_queue WHERE transaction_id = $1 AND status = 'pending';

-- name: UpdateReviewDecision :one
UPDATE review_queue
SET status = $2,
    reviewer_type = $3,
    reviewer_id = $4,
    reviewer_name = $5,
    review_note = $6,
    resolved_category_id = $7,
    reviewed_at = NOW()
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: DeleteReview :exec
DELETE FROM review_queue WHERE id = $1 AND status = 'pending';

-- name: DeleteAllPendingReviews :execrows
DELETE FROM review_queue WHERE status = 'pending';

-- name: CountPendingReviews :one
SELECT COUNT(*) FROM review_queue rq
JOIN transactions t ON rq.transaction_id = t.id
JOIN accounts a ON t.account_id = a.id
WHERE rq.status = 'pending'
  AND t.deleted_at IS NULL
  AND (a.is_dependent_linked = FALSE
       OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id));

-- name: CountReviewsByStatusToday :many
SELECT status, COUNT(*) as count
FROM review_queue
WHERE reviewed_at >= CURRENT_DATE
GROUP BY status;
