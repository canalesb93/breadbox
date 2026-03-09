-- name: InsertCategory :one
INSERT INTO categories (slug, display_name, parent_id, icon, color, sort_order, is_system, hidden)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetCategoryByID :one
SELECT * FROM categories WHERE id = $1;

-- name: GetCategoryBySlug :one
SELECT * FROM categories WHERE slug = $1;

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

-- name: ReassignMappingsCategory :exec
UPDATE category_mappings
SET category_id = $2, updated_at = NOW()
WHERE category_id = $1;

-- name: SetTransactionCategoryOverride :exec
UPDATE transactions
SET category_id = $2, category_override = TRUE, updated_at = NOW()
WHERE id = $1;

-- name: ClearTransactionCategoryOverride :exec
UPDATE transactions
SET category_override = FALSE, updated_at = NOW()
WHERE id = $1;

-- name: ListUnmappedCategories :many
SELECT DISTINCT bc.provider, t.category_primary, t.category_detailed
FROM transactions t
JOIN accounts a ON t.account_id = a.id
JOIN bank_connections bc ON a.connection_id = bc.id
WHERE t.category_id = (SELECT id FROM categories WHERE slug = 'uncategorized')
  AND t.category_override = FALSE
  AND t.deleted_at IS NULL
  AND (t.category_primary IS NOT NULL OR t.category_detailed IS NOT NULL)
ORDER BY bc.provider, t.category_primary, t.category_detailed;
