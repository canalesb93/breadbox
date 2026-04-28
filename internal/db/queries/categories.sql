-- name: InsertCategory :one
INSERT INTO categories (slug, display_name, parent_id, icon, color, sort_order, is_system, hidden)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetCategoryByID :one
SELECT * FROM categories WHERE id = $1;

-- name: GetCategoryBySlug :one
SELECT * FROM categories WHERE slug = $1;

-- name: GetCategoryUUIDByShortID :one
SELECT id FROM categories WHERE short_id = $1;

-- name: ListCategories :many
SELECT c.*, p.slug AS parent_slug, p.display_name AS parent_display_name
FROM categories c
LEFT JOIN categories p ON c.parent_id = p.id
ORDER BY c.sort_order, c.display_name;

-- name: ListCategoriesByParent :many
SELECT * FROM categories
WHERE parent_id = $1
ORDER BY sort_order, display_name;

-- name: ListTopLevelCategories :many
SELECT * FROM categories
WHERE parent_id IS NULL
ORDER BY sort_order, display_name;

-- name: UpdateCategory :one
UPDATE categories
SET display_name = $2, icon = $3, color = $4, sort_order = $5, hidden = $6, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteCategory :exec
DELETE FROM categories WHERE id = $1;

-- name: CountTransactionsByCategory :one
SELECT COUNT(*) FROM transactions
WHERE category_id = $1 AND deleted_at IS NULL;

-- name: ListChildCategoryIDs :many
SELECT id FROM categories WHERE parent_id = $1;

-- name: ReassignTransactionsCategory :exec
UPDATE transactions
SET category_id = $2, updated_at = NOW()
WHERE category_id = $1 AND deleted_at IS NULL;

-- name: SetTransactionCategoryOverride :execrows
UPDATE transactions
SET category_id = $2, category_override = TRUE, updated_at = NOW()
WHERE id = $1;

-- name: ClearTransactionCategoryOverride :execrows
UPDATE transactions
SET category_override = FALSE, updated_at = NOW()
WHERE id = $1;

-- name: SetCategoryOverrideFlag :execrows
-- Flips the override flag without changing the category. Used by the
-- detail-page lock toggle, which only governs whether transaction rules
-- may re-categorize the row.
UPDATE transactions
SET category_override = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- ReassignRulesCategory was removed in 20260415070000_rule_actions_v2 — rule
-- category targets now live inside the actions JSONB. Reassignment is handled
-- in the service layer via pool.Exec to keep sqlc happy with JSONB subqueries.

