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

-- name: CountUsers :one
SELECT count(*) FROM users;
