-- name: CreateOAuthClient :one
INSERT INTO oauth_clients (name, client_id, client_secret_hash, client_id_prefix, redirect_uris, scope)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GetOAuthClientByClientID :one
SELECT * FROM oauth_clients WHERE client_id = $1;

-- name: ListOAuthClients :many
SELECT * FROM oauth_clients ORDER BY created_at DESC;

-- name: RevokeOAuthClient :exec
UPDATE oauth_clients SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL;

-- name: CreateOAuthAuthorizationCode :exec
INSERT INTO oauth_authorization_codes (code_hash, client_id, admin_id, redirect_uri, scope, code_challenge, code_challenge_method, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetOAuthAuthorizationCode :one
SELECT * FROM oauth_authorization_codes WHERE code_hash = $1;

-- name: MarkOAuthAuthorizationCodeUsed :exec
UPDATE oauth_authorization_codes SET used_at = NOW() WHERE code_hash = $1;

-- name: CreateOAuthAccessToken :one
INSERT INTO oauth_access_tokens (token_hash, client_id, admin_id, scope, expires_at)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetOAuthAccessTokenByHash :one
SELECT * FROM oauth_access_tokens WHERE token_hash = $1;

-- name: UpdateOAuthAccessTokenLastUsed :exec
UPDATE oauth_access_tokens SET last_used_at = NOW() WHERE id = $1;

-- name: RevokeOAuthAccessTokensByClient :exec
UPDATE oauth_access_tokens SET revoked_at = NOW() WHERE client_id = $1 AND revoked_at IS NULL;

-- name: CreateOAuthRefreshToken :one
INSERT INTO oauth_refresh_tokens (token_hash, access_token_id, expires_at)
VALUES ($1, $2, $3) RETURNING *;

-- name: GetOAuthRefreshTokenByHash :one
SELECT * FROM oauth_refresh_tokens WHERE token_hash = $1;

-- name: RevokeOAuthRefreshToken :exec
UPDATE oauth_refresh_tokens SET revoked_at = NOW() WHERE id = $1;

-- name: RevokeOAuthRefreshTokensByAccessToken :exec
UPDATE oauth_refresh_tokens SET revoked_at = NOW() WHERE access_token_id = $1 AND revoked_at IS NULL;

-- name: DeleteExpiredOAuthAuthorizationCodes :exec
DELETE FROM oauth_authorization_codes WHERE expires_at < NOW();
