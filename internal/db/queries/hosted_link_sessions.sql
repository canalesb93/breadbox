-- name: CreateHostedLinkSession :one
INSERT INTO hosted_link_sessions (
    token_hash,
    user_id,
    provider,
    action,
    connection_id,
    single_use,
    redirect_url,
    label,
    expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetHostedLinkSessionByID :one
SELECT * FROM hosted_link_sessions WHERE id = $1;

-- name: GetHostedLinkSessionByShortID :one
SELECT * FROM hosted_link_sessions WHERE short_id = $1;

-- name: GetHostedLinkSessionByTokenHash :one
-- Used by the bearer middleware (PR3) to resolve a session from the
-- plaintext token without ever storing the plaintext at rest.
SELECT * FROM hosted_link_sessions WHERE token_hash = $1;

-- name: UpdateHostedLinkSessionStatus :one
-- Single status-transition write that the service composes around. Pass
-- nulls (e.g. via pgtype.Timestamptz{}) for any field that should not
-- change; passing a value overwrites it (including clearing an error by
-- writing empty strings).
UPDATE hosted_link_sessions
SET status        = $2,
    error_code    = $3,
    error_message = $4,
    started_at    = COALESCE($5, started_at),
    completed_at  = COALESCE($6, completed_at)
WHERE id = $1
RETURNING *;

-- name: AppendHostedLinkSessionResult :exec
-- Append a newly created bank_connections.id to result_connection_ids.
-- array_append is idempotent in shape (no de-dup); the service guards
-- against duplicate appends if/when needed.
UPDATE hosted_link_sessions
SET result_connection_ids = array_append(result_connection_ids, sqlc.arg(connection_id)::uuid)
WHERE id = sqlc.arg(id);

-- name: ExpireHostedLinkSessions :execrows
-- Bulk-expire pending/active sessions whose TTL has elapsed. Returns the
-- number of rows updated so a cleanup job can log it. Idempotent — running
-- twice in a row yields 0 the second time.
UPDATE hosted_link_sessions
SET status = 'expired'
WHERE expires_at < NOW()
  AND status IN ('pending', 'active');
