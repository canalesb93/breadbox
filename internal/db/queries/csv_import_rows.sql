-- name: CreateCSVImportRows :copyfrom
-- Bulk-insert fully-parsed + classified rows (up to 50k). Classification is
-- computed in Go, so (re)classification is a DeleteCSVImportRows + this copy.
INSERT INTO csv_import_rows (
    session_id, row_index, raw, parsed_date, parsed_amount, parsed_desc,
    parsed_merchant, parsed_category, classification, match_txn_id, match_score,
    match_reason, parse_error, content_hash, provider_txn_id, include, user_edited,
    category_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
);

-- name: DeleteCSVImportRows :exec
DELETE FROM csv_import_rows WHERE session_id = $1;

-- name: GetCSVImportRow :one
SELECT * FROM csv_import_rows WHERE id = $1;

-- name: GetCSVImportRowsPage :many
-- Paginated preview. Empty $2 returns all classifications.
SELECT * FROM csv_import_rows
WHERE session_id = $1
  AND ($2 = '' OR classification = $2)
ORDER BY row_index
LIMIT $3 OFFSET $4;

-- name: CountCSVImportRowsByClassification :many
SELECT classification, COUNT(*) AS count
FROM csv_import_rows
WHERE session_id = $1
GROUP BY classification;

-- name: CountIncludedCSVImportRows :one
SELECT COUNT(*) FROM csv_import_rows
WHERE session_id = $1
  AND include = TRUE
  AND classification <> 'error'
  AND classification <> 'needs_account';

-- name: ListIncludedCSVImportRows :many
-- The exact set that apply will upsert: included, non-error rows in row order.
SELECT * FROM csv_import_rows
WHERE session_id = $1
  AND include = TRUE
  AND classification <> 'error'
  AND classification <> 'needs_account'
ORDER BY row_index;

-- name: UpdateCSVImportRow :one
-- Inline edit of a single row (also re-derives its hash/classification in Go).
UPDATE csv_import_rows
SET parsed_date     = $2,
    parsed_amount   = $3,
    parsed_desc     = $4,
    parsed_merchant = $5,
    parsed_category = $6,
    classification  = $7,
    match_txn_id    = $8,
    match_score     = $9,
    match_reason    = $10,
    parse_error     = $11,
    content_hash    = $12,
    provider_txn_id = $13,
    include         = $14,
    category_id     = $15,
    user_edited     = TRUE
WHERE id = $1
RETURNING *;

-- name: SetCSVImportRowInclude :exec
UPDATE csv_import_rows SET include = $2 WHERE id = $1;

-- name: SetCSVImportRowsIncludeByClassification :exec
UPDATE csv_import_rows
SET include = $3
WHERE session_id = $1 AND classification = $2;

-- name: SetCSVImportRowsCategoryAll :exec
-- Bulk "set category" across all included rows in the session.
UPDATE csv_import_rows
SET category_id = $2, user_edited = TRUE
WHERE session_id = $1 AND include = TRUE;
