-- name: CreateCSVImportSession :one
INSERT INTO csv_import_sessions (
    user_id, status, filename, file_sha256, delimiter, headers, raw_blob,
    row_count, detected_template, column_mapping, date_format,
    positive_is_debit, has_debit_credit, iso_currency_code, profile_id, expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11,
    $12, $13, $14, $15, $16
)
RETURNING *;

-- name: GetCSVImportSession :one
SELECT * FROM csv_import_sessions WHERE id = $1;

-- name: GetCSVImportSessionByShortID :one
SELECT * FROM csv_import_sessions WHERE short_id = $1;

-- name: UpdateCSVImportSessionStatus :exec
UPDATE csv_import_sessions
SET status = $2, updated_at = NOW()
WHERE id = $1;

-- name: ResolveCSVImportSessionAccount :one
-- Bind the chosen target account/connection + currency and advance status.
UPDATE csv_import_sessions
SET resolved_account_id    = $2,
    resolved_connection_id = $3,
    iso_currency_code      = $4,
    status                 = $5,
    updated_at             = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateCSVImportSessionParse :one
-- Re-map columns / sign / date-format / template (triggers a reclassify).
UPDATE csv_import_sessions
SET column_mapping    = $2,
    date_format       = $3,
    positive_is_debit = $4,
    has_debit_credit  = $5,
    detected_template = $6,
    updated_at        = NOW()
WHERE id = $1
RETURNING *;

-- name: SetCSVImportSessionProfile :exec
UPDATE csv_import_sessions
SET profile_id = $2, updated_at = NOW()
WHERE id = $1;

-- name: FinalizeCSVImportSession :exec
-- After a successful apply: snapshot result, link the sync log, drop the blob.
UPDATE csv_import_sessions
SET status      = 'applied',
    result      = $2,
    sync_log_id = $3,
    raw_blob    = NULL,
    updated_at  = NOW()
WHERE id = $1;

-- name: ListExpiredCSVImportSessions :many
SELECT * FROM csv_import_sessions
WHERE status <> 'applied' AND expires_at IS NOT NULL AND expires_at < NOW()
ORDER BY expires_at;

-- name: DeleteCSVImportSession :exec
DELETE FROM csv_import_sessions WHERE id = $1;
