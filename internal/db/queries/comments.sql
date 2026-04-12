-- name: CreateComment :one
INSERT INTO transaction_comments (transaction_id, author_type, author_id, author_name, content, review_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListCommentsByTransaction :many
SELECT * FROM transaction_comments
WHERE transaction_id = $1
ORDER BY created_at ASC;

-- name: GetComment :one
SELECT * FROM transaction_comments WHERE id = $1;

-- name: GetCommentByReviewID :one
SELECT * FROM transaction_comments WHERE review_id = $1 LIMIT 1;

-- name: GetCommentUUIDByShortID :one
SELECT id FROM transaction_comments WHERE short_id = $1;

-- name: UpdateComment :one
UPDATE transaction_comments
SET content = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteComment :exec
DELETE FROM transaction_comments WHERE id = $1;

-- name: CountCommentsByTransaction :one
SELECT COUNT(*) FROM transaction_comments WHERE transaction_id = $1;
