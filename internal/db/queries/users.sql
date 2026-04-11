-- name: CreateUser :one
INSERT INTO users (name, email)
VALUES ($1, $2)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY name;

-- name: UpdateUser :one
UPDATE users SET name = $2, email = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: GetUserUUIDByShortID :one
SELECT id FROM users WHERE short_id = $1;

-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: ListUsersWithoutAuthAccount :many
SELECT u.* FROM users u
LEFT JOIN auth_accounts aa ON aa.user_id = u.id
WHERE aa.id IS NULL
ORDER BY u.name;

-- name: GetUserAvatar :one
SELECT avatar_data, avatar_content_type, avatar_seed, updated_at FROM users WHERE id = $1;

-- name: SetUserAvatarSeed :exec
UPDATE users SET avatar_seed = $2, updated_at = NOW()
WHERE id = $1;

-- name: SetUserAvatar :exec
UPDATE users SET avatar_data = $2, avatar_content_type = $3, updated_at = NOW()
WHERE id = $1;

-- name: ClearUserAvatar :exec
UPDATE users SET avatar_data = NULL, avatar_content_type = NULL, updated_at = NOW()
WHERE id = $1;
