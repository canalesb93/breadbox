-- name: CreateTransactionMatch :one
INSERT INTO transaction_matches (account_link_id, primary_transaction_id, dependent_transaction_id, match_confidence, matched_on)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: GetTransactionMatch :one
SELECT * FROM transaction_matches WHERE id = $1;

-- name: GetTransactionMatchByPrimaryTxn :one
SELECT * FROM transaction_matches WHERE primary_transaction_id = $1;

-- name: GetTransactionMatchByDependentTxn :one
SELECT * FROM transaction_matches WHERE dependent_transaction_id = $1;

-- name: ListTransactionMatchesByLink :many
SELECT tm.*,
       pt.name AS primary_txn_name, pt.amount AS primary_txn_amount, pt.date AS primary_txn_date, pt.merchant_name AS primary_txn_merchant,
       dt.name AS dependent_txn_name, dt.amount AS dependent_txn_amount, dt.date AS dependent_txn_date, dt.merchant_name AS dependent_txn_merchant
FROM transaction_matches tm
JOIN transactions pt ON tm.primary_transaction_id = pt.id
JOIN transactions dt ON tm.dependent_transaction_id = dt.id
WHERE tm.account_link_id = $1
ORDER BY pt.date DESC, tm.created_at DESC;

-- name: CountTransactionMatchesByLink :one
SELECT COUNT(*) FROM transaction_matches WHERE account_link_id = $1;

-- name: CountUnmatchedDependentTransactions :one
SELECT COUNT(*) FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE a.id = $1
  AND t.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id);

-- name: UpdateTransactionMatchConfidence :one
UPDATE transaction_matches SET match_confidence = $2 WHERE id = $1 RETURNING *;

-- name: DeleteTransactionMatch :exec
DELETE FROM transaction_matches WHERE id = $1;

-- name: DeleteTransactionMatchesByLink :exec
DELETE FROM transaction_matches WHERE account_link_id = $1;

-- name: SetTransactionAttributedUser :exec
UPDATE transactions SET attributed_user_id = $2, updated_at = NOW() WHERE id = $1;

-- name: ClearTransactionAttributedUserByLink :exec
UPDATE transactions t SET attributed_user_id = NULL, updated_at = NOW()
FROM transaction_matches tm
WHERE tm.primary_transaction_id = t.id AND tm.account_link_id = $1;
