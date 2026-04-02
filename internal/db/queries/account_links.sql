-- name: CreateAccountLink :one
INSERT INTO account_links (primary_account_id, dependent_account_id, match_strategy, match_tolerance_days)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAccountLinkUUIDByShortID :one
SELECT id FROM account_links WHERE short_id = $1;

-- name: GetAccountLink :one
SELECT al.*,
       pa.name AS primary_account_name,
       COALESCE(pa.display_name, pa.name) AS primary_account_display_name,
       da.name AS dependent_account_name,
       COALESCE(da.display_name, da.name) AS dependent_account_display_name,
       pu.name AS primary_user_name,
       du.name AS dependent_user_name
FROM account_links al
JOIN accounts pa ON al.primary_account_id = pa.id
JOIN accounts da ON al.dependent_account_id = da.id
LEFT JOIN bank_connections pbc ON pa.connection_id = pbc.id
LEFT JOIN users pu ON pbc.user_id = pu.id
LEFT JOIN bank_connections dbc ON da.connection_id = dbc.id
LEFT JOIN users du ON dbc.user_id = du.id
WHERE al.id = $1;

-- name: ListAccountLinks :many
SELECT al.*,
       pa.name AS primary_account_name,
       COALESCE(pa.display_name, pa.name) AS primary_account_display_name,
       da.name AS dependent_account_name,
       COALESCE(da.display_name, da.name) AS dependent_account_display_name,
       pu.name AS primary_user_name,
       du.name AS dependent_user_name
FROM account_links al
JOIN accounts pa ON al.primary_account_id = pa.id
JOIN accounts da ON al.dependent_account_id = da.id
LEFT JOIN bank_connections pbc ON pa.connection_id = pbc.id
LEFT JOIN users pu ON pbc.user_id = pu.id
LEFT JOIN bank_connections dbc ON da.connection_id = dbc.id
LEFT JOIN users du ON dbc.user_id = du.id
ORDER BY al.created_at DESC;

-- name: UpdateAccountLink :one
UPDATE account_links
SET match_strategy = $2,
    match_tolerance_days = $3,
    enabled = $4,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteAccountLink :exec
DELETE FROM account_links WHERE id = $1;

-- name: ListAccountLinksByAccountID :many
SELECT * FROM account_links
WHERE (primary_account_id = $1 OR dependent_account_id = $1) AND enabled = TRUE;

-- name: ListAccountLinksByConnectionID :many
SELECT DISTINCT al.* FROM account_links al
JOIN accounts a ON a.id = al.primary_account_id OR a.id = al.dependent_account_id
WHERE a.connection_id = $1 AND al.enabled = TRUE;

-- name: AccountLinkExists :one
SELECT EXISTS(
    SELECT 1 FROM account_links
    WHERE primary_account_id = $1 AND dependent_account_id = $2
) AS exists;

-- name: UpdateAccountDependentLinked :exec
UPDATE accounts SET is_dependent_linked = $2, updated_at = NOW() WHERE id = $1;

-- name: GetDependentUserID :one
SELECT u.id FROM accounts a
JOIN bank_connections bc ON a.connection_id = bc.id
JOIN users u ON bc.user_id = u.id
WHERE a.id = $1;
