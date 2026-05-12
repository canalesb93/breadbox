-- name: CreateAuthDeviceCode :one
INSERT INTO auth_device_codes (device_code, user_code, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetAuthDeviceCodeByDeviceCode :one
SELECT * FROM auth_device_codes WHERE device_code = $1;

-- name: GetAuthDeviceCodeByUserCode :one
SELECT * FROM auth_device_codes WHERE user_code = $1;

-- name: ApproveAuthDeviceCode :one
UPDATE auth_device_codes
   SET status = 'approved',
       api_key_id = $2,
       api_key_secret = $3,
       approved_at = NOW(),
       approved_by = $4
 WHERE id = $1 AND status = 'pending'
 RETURNING *;

-- name: DenyAuthDeviceCode :one
UPDATE auth_device_codes
   SET status = 'denied',
       approved_at = NOW(),
       approved_by = $2
 WHERE id = $1 AND status = 'pending'
 RETURNING *;

-- name: ExpireAuthDeviceCode :exec
UPDATE auth_device_codes
   SET status = 'expired'
 WHERE id = $1 AND status = 'pending';

-- name: ClearAuthDeviceCodeSecret :exec
UPDATE auth_device_codes
   SET api_key_secret = NULL
 WHERE id = $1;
