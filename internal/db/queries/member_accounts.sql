-- name: CreateMemberAccount :one
INSERT INTO member_accounts (user_id, username, hashed_password, role)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetMemberAccountByUsername :one
SELECT * FROM member_accounts WHERE username = $1;

-- name: GetMemberAccountByID :one
SELECT * FROM member_accounts WHERE id = $1;

-- name: GetMemberAccountByUserID :one
SELECT * FROM member_accounts WHERE user_id = $1;

-- name: ListMemberAccounts :many
SELECT ma.*, u.name as user_name, u.email as user_email
FROM member_accounts ma
JOIN users u ON u.id = ma.user_id
ORDER BY u.name;

-- name: UpdateMemberAccountPassword :exec
UPDATE member_accounts SET hashed_password = $2, updated_at = NOW() WHERE id = $1;

-- name: UpdateMemberAccountRole :exec
UPDATE member_accounts SET role = $2, updated_at = NOW() WHERE id = $1;

-- name: DeleteMemberAccount :exec
DELETE FROM member_accounts WHERE id = $1;

-- name: CountMemberAccounts :one
SELECT COUNT(*) FROM member_accounts;
