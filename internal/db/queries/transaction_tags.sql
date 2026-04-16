-- name: AddTransactionTag :execrows
INSERT INTO transaction_tags (transaction_id, tag_id, added_by_type, added_by_id, added_by_name)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (transaction_id, tag_id) DO NOTHING;

-- name: RemoveTransactionTag :execrows
DELETE FROM transaction_tags
WHERE transaction_id = $1 AND tag_id = $2;

-- name: ListTagsByTransaction :many
SELECT t.*
FROM tags t
JOIN transaction_tags tt ON tt.tag_id = t.id
WHERE tt.transaction_id = $1
ORDER BY tt.added_at ASC;

-- name: ListTagSlugsByTransaction :many
SELECT t.slug
FROM tags t
JOIN transaction_tags tt ON tt.tag_id = t.id
WHERE tt.transaction_id = $1
ORDER BY t.slug ASC;

-- name: CountTransactionsWithTagSlug :one
-- Returns how many non-deleted, non-matched-dependent transactions currently
-- carry the given tag slug. Used by the admin nav "pending reviews" badge and
-- dashboard card after Phase 3 retired review_queue.
SELECT COUNT(*)
FROM transaction_tags tt
JOIN tags tag ON tag.id = tt.tag_id
JOIN transactions t ON t.id = tt.transaction_id
JOIN accounts a ON a.id = t.account_id
WHERE tag.slug = $1
  AND t.deleted_at IS NULL
  AND (a.is_dependent_linked = FALSE
       OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id));
