-- name: InsertWebhookDelivery :one
INSERT INTO webhook_deliveries (event, url, payload, delivery_id, next_retry_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateWebhookDeliveryAttempt :exec
UPDATE webhook_deliveries
SET status = $2,
    attempts = attempts + 1,
    last_attempt_at = NOW(),
    next_retry_at = $3,
    response_status = $4,
    response_body = $5,
    error_message = $6
WHERE id = $1;

-- name: ListRecentWebhookDeliveries :many
SELECT * FROM webhook_deliveries
ORDER BY created_at DESC
LIMIT $1;

-- name: GetPendingWebhookDeliveries :many
SELECT * FROM webhook_deliveries
WHERE status = 'pending'
  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
ORDER BY created_at ASC
LIMIT 100;

-- name: CleanupOldWebhookDeliveries :exec
DELETE FROM webhook_deliveries
WHERE created_at < NOW() - INTERVAL '7 days';
