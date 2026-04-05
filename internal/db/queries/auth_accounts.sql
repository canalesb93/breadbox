-- name: CreateAuthAccount :one
INSERT INTO auth_accounts (user_id, username, hashed_password, role)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAuthAccountByUsername :one
SELECT * FROM auth_accounts WHERE username = $1;

-- name: GetAuthAccountByID :one
SELECT * FROM auth_accounts WHERE id = $1;

-- name: GetAuthAccountByUserID :one
SELECT * FROM auth_accounts WHERE user_id = $1;

-- name: CountAuthAccounts :one
SELECT COUNT(*) FROM auth_accounts;

-- name: CountAuthAdminAccounts :one
SELECT COUNT(*) FROM auth_accounts WHERE role = 'admin';

-- name: ListAuthAccounts :many
SELECT aa.*, u.name as user_name, u.email as user_email
FROM auth_accounts aa
LEFT JOIN users u ON u.id = aa.user_id
ORDER BY COALESCE(u.name, aa.username);

-- name: ListAuthAccountsWithUser :many
SELECT aa.*, u.name as user_name, u.email as user_email
FROM auth_accounts aa
JOIN users u ON u.id = aa.user_id
ORDER BY u.name;

-- name: UpdateAuthAccountPassword :exec
UPDATE auth_accounts SET hashed_password = $2, updated_at = NOW() WHERE id = $1;

-- name: UpdateAuthAccountRole :exec
UPDATE auth_accounts SET role = $2, updated_at = NOW() WHERE id = $1;

-- name: DeleteAuthAccount :exec
DELETE FROM auth_accounts WHERE id = $1;
