-- name: InsertSyncLogAccount :exec
INSERT INTO sync_log_accounts (sync_log_id, account_id, account_name, added_count, modified_count, removed_count)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (sync_log_id, account_id) DO UPDATE SET
    added_count = sync_log_accounts.added_count + EXCLUDED.added_count,
    modified_count = sync_log_accounts.modified_count + EXCLUDED.modified_count,
    removed_count = sync_log_accounts.removed_count + EXCLUDED.removed_count;

-- name: ListSyncLogAccounts :many
SELECT sla.id, sla.sync_log_id, sla.account_id, sla.account_name,
       sla.added_count, sla.modified_count, sla.removed_count
FROM sync_log_accounts sla
WHERE sla.sync_log_id = $1
ORDER BY (sla.added_count + sla.modified_count + sla.removed_count) DESC, sla.account_name;

-- name: CountAffectedAccountsBySyncLog :one
SELECT COUNT(*) FROM sync_log_accounts WHERE sync_log_id = $1;

-- name: CountAffectedAccountsBySyncLogIDs :many
SELECT sync_log_id, COUNT(*) AS account_count
FROM sync_log_accounts
WHERE sync_log_id = ANY($1::uuid[])
GROUP BY sync_log_id;
