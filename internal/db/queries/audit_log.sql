-- name: InsertAuditLogEntry :exec
INSERT INTO audit_log (entity_type, entity_id, action, field, old_value, new_value, actor_type, actor_id, actor_name, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);
