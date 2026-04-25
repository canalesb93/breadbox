-- name: InsertAnnotation :one
INSERT INTO annotations (
    transaction_id, kind, actor_type, actor_id, actor_name,
    session_id, payload, tag_id, rule_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListAnnotationsByTransaction :many
SELECT * FROM annotations
WHERE transaction_id = $1
ORDER BY created_at ASC;

-- name: ListAnnotationsWithActorByTransaction :many
-- Carries the actor user's updated_at as a unix-timestamp avatar version so
-- the activity timeline can cache-bust avatar URLs (`?v=<ts>`) and pick up
-- new uploads immediately rather than serving stale bytes within the avatar
-- handler's 24h max-age window. annotations.actor_id can hold either a
-- users.id or an auth_accounts.id depending on the call site; resolve both
-- by joining auth_accounts and falling back to a direct users join.
SELECT
    a.id, a.short_id, a.transaction_id, a.kind, a.actor_type,
    a.actor_id, a.actor_name, a.session_id, a.payload, a.tag_id,
    a.rule_id, a.created_at,
    COALESCE(u_via_account.updated_at, u_direct.updated_at) AS actor_updated_at
FROM annotations a
LEFT JOIN auth_accounts aa
    ON a.actor_type = 'user'
   AND a.actor_id IS NOT NULL
   AND a.actor_id::uuid = aa.id
LEFT JOIN users u_via_account
    ON aa.user_id = u_via_account.id
LEFT JOIN users u_direct
    ON a.actor_type = 'user'
   AND a.actor_id IS NOT NULL
   AND aa.id IS NULL
   AND a.actor_id::uuid = u_direct.id
WHERE a.transaction_id = $1
ORDER BY a.created_at ASC;

-- name: CountAnnotationsByTransactionAndKind :one
SELECT COUNT(*) FROM annotations
WHERE transaction_id = $1 AND kind = $2;

-- name: GetAnnotationByID :one
SELECT * FROM annotations WHERE id = $1;

-- name: GetAnnotationUUIDByShortID :one
SELECT id FROM annotations WHERE short_id = $1;

-- name: UpdateAnnotationPayload :one
UPDATE annotations
SET payload = $2
WHERE id = $1
RETURNING *;

-- name: DeleteAnnotation :exec
DELETE FROM annotations WHERE id = $1;
