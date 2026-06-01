-- name: InsertRecurringSeries :one
INSERT INTO recurring_series (
    user_id, name, merchant_key, cadence, expected_day, expected_amount,
    amount_tolerance, iso_currency_code, category_id, status, detection_source,
    confidence, confirmed_by_type, last_amount, last_seen_date, next_expected_date,
    occurrence_count, detection_signals
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
)
RETURNING *;

-- name: GetRecurringSeriesByID :one
SELECT * FROM recurring_series WHERE id = $1 AND deleted_at IS NULL;

-- name: GetRecurringSeriesUUIDByShortID :one
SELECT id FROM recurring_series WHERE short_id = $1;

-- MatchSeriesForUpdate finds the live series matching the dedup signature and
-- row-locks it so concurrent detector/agent writers converge. NULL-safe on
-- currency + user (IS NOT DISTINCT FROM). Prefers a non-cancelled row, then the
-- oldest, so resubscribe reactivates the original identity rather than forking.
-- name: MatchSeriesForUpdate :one
SELECT * FROM recurring_series
WHERE merchant_key = sqlc.arg('merchant_key')
  AND iso_currency_code IS NOT DISTINCT FROM sqlc.narg('iso_currency_code')
  AND user_id IS NOT DISTINCT FROM sqlc.narg('user_id')
  AND deleted_at IS NULL
ORDER BY (status <> 'cancelled') DESC, created_at ASC
LIMIT 1
FOR UPDATE;

-- name: UpdateRecurringSeries :one
UPDATE recurring_series
SET user_id = $2,
    name = $3,
    merchant_key = $4,
    cadence = $5,
    expected_day = $6,
    expected_amount = $7,
    amount_tolerance = $8,
    iso_currency_code = $9,
    category_id = $10,
    status = $11,
    detection_source = $12,
    confidence = $13,
    confirmed_by_type = $14,
    last_amount = $15,
    last_seen_date = $16,
    next_expected_date = $17,
    occurrence_count = $18,
    detection_signals = $19,
    type = $20,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- SeriesDominantMemberCategory returns the most common category slug among a
-- series' live members — used to infer the series type at first detection.
-- Returns no rows when every member is uncategorized.
-- name: SeriesDominantMemberCategory :one
SELECT c.slug
FROM transactions t
JOIN categories c ON t.category_id = c.id
WHERE t.series_id = $1 AND t.deleted_at IS NULL
GROUP BY c.slug
ORDER BY COUNT(*) DESC, c.slug ASC
LIMIT 1;

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

-- SeriesMemberRollup recomputes occurrence_count / last_seen_date / last_amount
-- from the series' live members. Returns zero rows when the series has none.
-- name: SeriesMemberRollup :one
SELECT
    (COUNT(*) OVER ())::bigint AS occurrence_count,
    (MAX(date) OVER ())::date  AS last_seen_date,
    amount                     AS last_amount
FROM transactions
WHERE series_id = $1 AND deleted_at IS NULL
ORDER BY date DESC, id DESC
LIMIT 1;

-- name: ListRecurringSeriesByStatus :many
SELECT * FROM recurring_series
WHERE deleted_at IS NULL AND status = $1
ORDER BY created_at DESC;

-- name: ListRecurringSeries :many
SELECT * FROM recurring_series
WHERE deleted_at IS NULL
ORDER BY (status = 'candidate') DESC, occurrence_count DESC, created_at DESC;

-- name: CountRecurringSeries :one
SELECT COUNT(*) FROM recurring_series WHERE deleted_at IS NULL;

-- name: CountCandidateSeriesForReview :one
-- Candidates awaiting a human verdict, matching the /subscriptions "Needs
-- review" section: status='candidate' but NOT sticky-rejected (a rejected row
-- keeps status='candidate' with confidence='rejected' and is hidden).
SELECT COUNT(*) FROM recurring_series
WHERE deleted_at IS NULL AND status = 'candidate' AND confidence <> 'rejected';

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
