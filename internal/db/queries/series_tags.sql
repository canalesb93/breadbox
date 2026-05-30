-- name: AddSeriesTag :execrows
INSERT INTO series_tags (series_id, tag_id)
VALUES ($1, $2)
ON CONFLICT (series_id, tag_id) DO NOTHING;

-- name: RemoveSeriesTag :execrows
DELETE FROM series_tags WHERE series_id = $1 AND tag_id = $2;

-- name: ListSeriesTagSlugs :many
SELECT t.slug
FROM tags t
JOIN series_tags st ON st.tag_id = t.id
WHERE st.series_id = $1
ORDER BY t.slug;

-- name: ListSeriesTagIDs :many
SELECT tag_id FROM series_tags WHERE series_id = $1;

-- ApplySeriesTagsToTransactions materializes ALL of a series' tags onto the
-- given transactions (the just-linked members). Provenance marks them as
-- series-inherited so RemoveSeriesTagFromMembers can strip exactly these.
-- name: ApplySeriesTagsToTransactions :exec
INSERT INTO transaction_tags (transaction_id, tag_id, added_by_type, added_by_id, added_by_name)
SELECT u.txn_id, st.tag_id, 'system', s.short_id, s.name
FROM series_tags st
JOIN recurring_series s ON s.id = st.series_id
CROSS JOIN unnest($2::uuid[]) AS u(txn_id)
WHERE st.series_id = $1
ON CONFLICT (transaction_id, tag_id) DO NOTHING;

-- ApplySeriesTagToAllMembers materializes ONE tag onto every current member of
-- a series (used when a tag is newly added to the series — backfill).
-- name: ApplySeriesTagToAllMembers :exec
INSERT INTO transaction_tags (transaction_id, tag_id, added_by_type, added_by_id, added_by_name)
SELECT t.id, $2, 'system', s.short_id, s.name
FROM transactions t
JOIN recurring_series s ON s.id = $1
WHERE t.series_id = $1 AND t.deleted_at IS NULL
ON CONFLICT (transaction_id, tag_id) DO NOTHING;

-- RemoveSeriesTagFromMembers strips a series-inherited tag from all members.
-- Scoped by provenance (added_by_type='system' + added_by_id=series short_id)
-- so a tag the user added manually to a member survives.
-- name: RemoveSeriesTagFromMembers :exec
DELETE FROM transaction_tags
WHERE tag_id = $1 AND added_by_type = 'system' AND added_by_id = $2;
