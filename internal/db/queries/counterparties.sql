-- name: InsertCounterparty :one
INSERT INTO counterparties (name, website_url, logo_url, category_id, mcc, attrs)
VALUES ($1, $2, $3, $4, $5, COALESCE(sqlc.narg('attrs'), '{}'::jsonb))
RETURNING *;

-- name: GetCounterpartyByID :one
SELECT * FROM counterparties WHERE id = $1 AND deleted_at IS NULL;

-- name: GetCounterpartyByShortID :one
SELECT * FROM counterparties WHERE short_id = $1 AND deleted_at IS NULL;

-- name: GetCounterpartyUUIDByShortID :one
SELECT id FROM counterparties WHERE short_id = $1;

-- GetCounterpartyByName resolves a live counterparty by exact name. There is NO
-- UNIQUE on name (counterparties are assigned by short_id, not minted-by-name),
-- so this is the resolve half of the rule path's resolve-or-create: it picks the
-- oldest live row with the name, and the caller creates one only when none
-- exists. Deterministic (created_at ASC) so concurrent callers converge.
-- name: GetCounterpartyByName :one
SELECT * FROM counterparties
WHERE name = $1 AND deleted_at IS NULL
ORDER BY created_at ASC
LIMIT 1;

-- name: ListCounterparties :many
SELECT * FROM counterparties
WHERE deleted_at IS NULL
ORDER BY name ASC;

-- UpdateCounterparty applies the enrichment-lane edits. NULL args leave a column
-- unchanged via COALESCE, except name which is always set (the caller passes the
-- current value when unchanged).
-- name: UpdateCounterparty :one
UPDATE counterparties
SET name        = $2,
    website_url = COALESCE(sqlc.narg('website_url'), website_url),
    logo_url    = COALESCE(sqlc.narg('logo_url'), logo_url),
    category_id = COALESCE(sqlc.narg('category_id'), category_id),
    mcc         = COALESCE(sqlc.narg('mcc'), mcc),
    attrs       = COALESCE(sqlc.narg('attrs'), attrs),
    updated_at  = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteCounterparty :execrows
UPDATE counterparties
SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- LinkTransactionCounterparty attaches a transaction to a counterparty, NULL-fill
-- only — it never clobbers an existing assignment. Returns rows affected.
-- name: LinkTransactionCounterparty :execrows
UPDATE transactions
SET counterparty_id = $1, updated_at = NOW()
WHERE id = ANY(sqlc.arg('transaction_ids')::uuid[])
  AND counterparty_id IS NULL
  AND deleted_at IS NULL;

-- UnlinkTransactionCounterparty detaches transactions from a counterparty (clears
-- counterparty_id), guarded on counterparty_id so it can never steal a charge
-- from another counterparty. Returns rows affected so the caller can verify every
-- id was actually linked.
-- name: UnlinkTransactionCounterparty :execrows
UPDATE transactions
SET counterparty_id = NULL, updated_at = NOW()
WHERE id = ANY(sqlc.arg('transaction_ids')::uuid[])
  AND counterparty_id = sqlc.arg('counterparty_id')
  AND deleted_at IS NULL;

-- CounterpartyTransactionCount returns the live charge count linked to a
-- counterparty (the admin list / detail "N transactions" label).
-- name: CounterpartyTransactionCount :one
SELECT COUNT(*) FROM transactions
WHERE counterparty_id = $1 AND deleted_at IS NULL;

-- ListCounterpartyMemberShortIDs returns the short_ids of a counterparty's live
-- linked charges, newest first — fed to GetAdminTransactionRowsByIDs so the admin
-- detail page renders them through the shared transaction-row component.
-- name: ListCounterpartyMemberShortIDs :many
SELECT t.short_id
FROM transactions t
WHERE t.counterparty_id = $1 AND t.deleted_at IS NULL
ORDER BY t.date DESC, t.created_at DESC;

-- ListCounterpartyMemberCounts returns the live charge count per counterparty in
-- one pass, for the admin list page (avoids an N+1 of CounterpartyTransactionCount).
-- name: ListCounterpartyMemberCounts :many
SELECT cp.id, COUNT(t.id)::bigint AS member_count
FROM counterparties cp
LEFT JOIN transactions t ON t.counterparty_id = cp.id AND t.deleted_at IS NULL
WHERE cp.deleted_at IS NULL
GROUP BY cp.id;
