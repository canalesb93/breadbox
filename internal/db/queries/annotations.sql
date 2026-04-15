-- name: InsertAnnotation :one
INSERT INTO annotations (
    transaction_id, kind, actor_type, actor_id, actor_name,
    session_id, payload, tag_id, rule_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListAnnotationsByTransaction :many
SELECT * FROM annotations
WHERE transaction_id = $1
ORDER BY created_at ASC;

-- name: CountAnnotationsByTransactionAndKind :one
SELECT COUNT(*) FROM annotations
WHERE transaction_id = $1 AND kind = $2;
