-- name: InsertTag :one
INSERT INTO tags (slug, display_name, description, color, icon, lifecycle)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetTagByID :one
SELECT * FROM tags WHERE id = $1;

-- name: GetTagBySlug :one
SELECT * FROM tags WHERE slug = $1;

-- name: GetTagUUIDByShortID :one
SELECT id FROM tags WHERE short_id = $1;

-- name: ListTags :many
SELECT * FROM tags ORDER BY display_name;

-- name: UpdateTag :one
UPDATE tags
SET display_name = $2,
    description = $3,
    color = $4,
    icon = $5,
    lifecycle = $6,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteTag :execrows
DELETE FROM tags WHERE id = $1;
