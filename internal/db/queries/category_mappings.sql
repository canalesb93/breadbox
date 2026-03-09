-- name: InsertCategoryMapping :one
INSERT INTO category_mappings (provider, provider_category, category_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetCategoryMappingByID :one
SELECT * FROM category_mappings WHERE id = $1;

-- name: ListCategoryMappings :many
SELECT cm.*, c.slug AS category_slug, c.display_name AS category_display_name
FROM category_mappings cm
JOIN categories c ON cm.category_id = c.id
ORDER BY cm.provider, cm.provider_category;

-- name: ListCategoryMappingsByProvider :many
SELECT cm.*, c.slug AS category_slug, c.display_name AS category_display_name
FROM category_mappings cm
JOIN categories c ON cm.category_id = c.id
WHERE cm.provider = $1
ORDER BY cm.provider_category;

-- name: UpdateCategoryMapping :one
UPDATE category_mappings
SET category_id = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteCategoryMapping :exec
DELETE FROM category_mappings WHERE id = $1;

-- name: UpsertCategoryMapping :one
INSERT INTO category_mappings (provider, provider_category, category_id)
VALUES ($1, $2, $3)
ON CONFLICT (provider, provider_category) DO UPDATE
SET category_id = EXCLUDED.category_id, updated_at = NOW()
RETURNING *;

-- name: GetCategoryMappingByProviderCategory :one
SELECT * FROM category_mappings
WHERE provider = $1 AND provider_category = $2;

-- name: DeleteCategoryMappingByProviderCategory :exec
DELETE FROM category_mappings
WHERE provider = $1 AND provider_category = $2;

-- name: ListMappingsForResolver :many
SELECT provider_category, category_id
FROM category_mappings
WHERE provider = $1;
