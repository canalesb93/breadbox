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
