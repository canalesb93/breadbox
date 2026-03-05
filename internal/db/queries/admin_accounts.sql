-- name: CreateAdminAccount :one
INSERT INTO admin_accounts (username, hashed_password)
VALUES ($1, $2)
RETURNING *;

-- name: GetAdminAccountByUsername :one
SELECT * FROM admin_accounts WHERE username = $1;

-- name: CountAdminAccounts :one
SELECT COUNT(*) FROM admin_accounts;

-- name: GetAdminAccountByID :one
SELECT * FROM admin_accounts WHERE id = $1;

-- name: UpdateAdminPassword :exec
UPDATE admin_accounts SET hashed_password = $2 WHERE id = $1;
