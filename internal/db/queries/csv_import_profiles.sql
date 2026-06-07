-- name: UpsertCSVImportProfile :one
-- Create a profile for this (user, header layout), or update the existing one.
-- `name` is intentionally NOT in the conflict SET list so a user's rename
-- survives future imports. times_used is bumped and last_used_at refreshed.
INSERT INTO csv_import_profiles (
    user_id, name, header_fingerprint, headers, detected_template,
    column_mapping, date_format, delimiter, positive_is_debit, has_debit_credit,
    iso_currency_code, default_account_id, institution_hint, mask_hint,
    times_used, last_used_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13, $14,
    1, NOW()
)
ON CONFLICT (user_id, header_fingerprint) DO UPDATE SET
    headers           = EXCLUDED.headers,
    detected_template = EXCLUDED.detected_template,
    column_mapping    = EXCLUDED.column_mapping,
    date_format       = EXCLUDED.date_format,
    delimiter         = EXCLUDED.delimiter,
    positive_is_debit = EXCLUDED.positive_is_debit,
    has_debit_credit  = EXCLUDED.has_debit_credit,
    iso_currency_code = EXCLUDED.iso_currency_code,
    default_account_id = EXCLUDED.default_account_id,
    institution_hint  = EXCLUDED.institution_hint,
    mask_hint         = EXCLUDED.mask_hint,
    times_used        = csv_import_profiles.times_used + 1,
    last_used_at      = NOW(),
    updated_at        = NOW()
RETURNING *;

-- name: GetCSVImportProfileByFingerprint :one
SELECT * FROM csv_import_profiles
WHERE user_id = $1 AND header_fingerprint = $2;

-- name: GetCSVImportProfile :one
SELECT * FROM csv_import_profiles WHERE id = $1;

-- name: GetCSVImportProfileByShortID :one
SELECT * FROM csv_import_profiles WHERE short_id = $1;

-- name: ListCSVImportProfilesByUser :many
SELECT * FROM csv_import_profiles
WHERE user_id = $1
ORDER BY last_used_at DESC NULLS LAST, created_at DESC;

-- name: ListCSVImportProfiles :many
SELECT * FROM csv_import_profiles
ORDER BY last_used_at DESC NULLS LAST, created_at DESC;

-- name: RenameCSVImportProfile :one
UPDATE csv_import_profiles
SET name = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteCSVImportProfile :exec
DELETE FROM csv_import_profiles WHERE id = $1;
