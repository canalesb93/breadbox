-- name: CreateWebhookEvent :one
INSERT INTO webhook_events (provider, event_type, connection_id, raw_payload_hash, status, error_message)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateWebhookEventStatus :exec
UPDATE webhook_events SET status = $2, error_message = $3 WHERE id = $1;

-- name: CountWebhookEvents :one
SELECT COUNT(*) FROM webhook_events;
