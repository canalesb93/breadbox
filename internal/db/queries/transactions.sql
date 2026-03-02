-- name: CountTransactions :one
SELECT COUNT(*) FROM transactions WHERE deleted_at IS NULL;
