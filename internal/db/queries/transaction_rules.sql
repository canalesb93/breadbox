-- name: InsertTransactionRule :one
INSERT INTO transaction_rules (name, conditions, category_id, priority, enabled, expires_at, created_by_type, created_by_id, created_by_name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetRuleUUIDByShortID :one
SELECT id FROM transaction_rules WHERE short_id = $1;

-- name: GetTransactionRuleByID :one
SELECT tr.*,
       c.slug AS category_slug,
       c.display_name AS category_display_name
FROM transaction_rules tr
LEFT JOIN categories c ON tr.category_id = c.id
WHERE tr.id = $1;

-- name: UpdateTransactionRule :one
UPDATE transaction_rules
SET name = $2,
    conditions = $3,
    category_id = $4,
    priority = $5,
    enabled = $6,
    expires_at = $7,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteTransactionRule :execrows
DELETE FROM transaction_rules WHERE id = $1;

-- name: ListActiveRulesForSync :many
SELECT tr.*,
       c.slug AS category_slug,
       c.display_name AS category_display_name
FROM transaction_rules tr
LEFT JOIN categories c ON tr.category_id = c.id
WHERE tr.enabled = TRUE
  AND (tr.expires_at IS NULL OR tr.expires_at > NOW())
ORDER BY tr.priority DESC, tr.created_at DESC;

-- name: BatchIncrementHitCounts :exec
UPDATE transaction_rules
SET hit_count = hit_count + $2,
    last_hit_at = NOW()
WHERE id = $1;
