-- name: InsertRecurringSeries :one
INSERT INTO recurring_series (name, type)
VALUES ($1, $2)
RETURNING *;

-- UpsertRecurringSeriesByName is the surrogate-first mint: resolve a series by its
-- (live) name, creating it on first reference. Idempotent — a second assign_series
-- for the same name returns the same surrogate id. The DO UPDATE keeps the row's
-- updated_at fresh and lets RETURNING fire on the conflict path too.
-- name: UpsertRecurringSeriesByName :one
INSERT INTO recurring_series (name, type)
VALUES ($1, $2)
ON CONFLICT (name) WHERE deleted_at IS NULL
DO UPDATE SET updated_at = NOW()
RETURNING id;

-- name: GetRecurringSeriesByID :one
SELECT * FROM recurring_series WHERE id = $1 AND deleted_at IS NULL;

-- name: GetRecurringSeriesUUIDByShortID :one
SELECT id FROM recurring_series WHERE short_id = $1;

-- name: GetRecurringSeriesByName :one
SELECT * FROM recurring_series WHERE name = $1 AND deleted_at IS NULL;

-- name: UpdateRecurringSeries :one
UPDATE recurring_series
SET name = $2,
    type = $3,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: ListRecurringSeries :many
SELECT * FROM recurring_series
WHERE deleted_at IS NULL
ORDER BY name ASC;

-- name: CountRecurringSeries :one
SELECT COUNT(*) FROM recurring_series WHERE deleted_at IS NULL;

-- BackLinkSeriesMembers attaches the given transactions to a series, NULL-fill
-- only — it never clobbers a manual/rule assignment. Returns rows affected.
-- name: BackLinkSeriesMembers :execrows
UPDATE transactions
SET series_id = $1, updated_at = NOW()
WHERE id = ANY(sqlc.arg('transaction_ids')::uuid[])
  AND series_id IS NULL
  AND deleted_at IS NULL;

-- UnlinkSeriesMembers detaches the given transactions from a series (clears
-- series_id), guarded on series_id so it can never steal a charge from another
-- series. Returns rows affected so the caller can verify every id was a member.
-- name: UnlinkSeriesMembers :execrows
UPDATE transactions
SET series_id = NULL, updated_at = NOW()
WHERE id = ANY(sqlc.arg('transaction_ids')::uuid[])
  AND series_id = sqlc.arg('series_id')
  AND deleted_at IS NULL;

-- ListSeriesMembers returns a series' live member charges, newest first,
-- enriched with the category color/icon + pending flag + tag count the shared
-- transaction-row component renders (so linked charges look identical to the
-- /transactions list and the dashboard feed).
-- name: ListSeriesMembers :many
SELECT
    t.short_id,
    t.date,
    t.provider_name,
    t.provider_merchant_name,
    t.amount,
    t.iso_currency_code,
    t.pending,
    c.color AS category_color,
    c.icon  AS category_icon,
    (SELECT COUNT(*) FROM transaction_tags tt WHERE tt.transaction_id = t.id)::int AS tag_count
FROM transactions t
LEFT JOIN categories c ON c.id = t.category_id
WHERE t.series_id = $1 AND t.deleted_at IS NULL
ORDER BY t.date DESC, t.created_at DESC;
