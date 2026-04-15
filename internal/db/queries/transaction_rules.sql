-- name: InsertTransactionRule :one
INSERT INTO transaction_rules (
    name, conditions, actions, trigger, priority, enabled, expires_at,
    created_by_type, created_by_id, created_by_name
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetRuleUUIDByShortID :one
SELECT id FROM transaction_rules WHERE short_id = $1;

-- name: GetTransactionRuleByID :one
SELECT * FROM transaction_rules WHERE id = $1;

-- name: UpdateTransactionRule :one
UPDATE transaction_rules
SET name = $2,
    conditions = $3,
    actions = $4,
    trigger = $5,
    priority = $6,
    enabled = $7,
    expires_at = $8,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteTransactionRule :execrows
DELETE FROM transaction_rules WHERE id = $1;

-- name: ListActiveRulesForSync :many
SELECT id, conditions, actions, trigger
FROM transaction_rules
WHERE enabled = TRUE
  AND (expires_at IS NULL OR expires_at > NOW())
ORDER BY priority DESC, created_at DESC;

-- name: BatchIncrementHitCounts :exec
UPDATE transaction_rules
SET hit_count = hit_count + $2,
    last_hit_at = NOW()
WHERE id = $1;
