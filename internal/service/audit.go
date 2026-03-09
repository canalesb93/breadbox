package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// WriteAuditLog writes one or more audit log entries using a batch insert.
func (s *Service) WriteAuditLog(ctx context.Context, entries []AuditLogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// Build multi-row INSERT for efficiency.
	var sb strings.Builder
	sb.WriteString("INSERT INTO audit_log (entity_type, entity_id, action, field, old_value, new_value, actor_type, actor_id, actor_name, metadata) VALUES ")

	args := make([]any, 0, len(entries)*10)
	for i, e := range entries {
		if i > 0 {
			sb.WriteString(", ")
		}
		base := i * 10
		sb.WriteString(fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10))

		entityID, err := parseUUID(e.EntityID)
		if err != nil {
			return fmt.Errorf("invalid entity_id %q: %w", e.EntityID, err)
		}

		var field pgtype.Text
		if e.Field != nil {
			field = pgtype.Text{String: *e.Field, Valid: true}
		}
		var oldValue pgtype.Text
		if e.OldValue != nil {
			oldValue = pgtype.Text{String: *e.OldValue, Valid: true}
		}
		var newValue pgtype.Text
		if e.NewValue != nil {
			newValue = pgtype.Text{String: *e.NewValue, Valid: true}
		}
		var actorID pgtype.Text
		if e.Actor.ID != "" {
			actorID = pgtype.Text{String: e.Actor.ID, Valid: true}
		}

		var metadata []byte
		if len(e.Metadata) > 0 {
			metadata, _ = json.Marshal(e.Metadata)
		}

		args = append(args, e.EntityType, entityID, e.Action, field, oldValue, newValue,
			e.Actor.Type, actorID, e.Actor.Name, metadata)
	}

	_, err := s.Pool.Exec(ctx, sb.String(), args...)
	if err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}

// ListAuditLog returns audit log entries for a specific entity.
func (s *Service) ListAuditLog(ctx context.Context, params AuditLogListParams) (*AuditLogListResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	entityID, err := parseUUID(params.EntityID)
	if err != nil {
		return nil, fmt.Errorf("invalid entity_id: %w", err)
	}

	query := "SELECT id, entity_type, entity_id, action, field, old_value, new_value, actor_type, actor_id, actor_name, metadata, created_at FROM audit_log WHERE entity_type = $1 AND entity_id = $2"
	args := []any{params.EntityType, entityID}
	argN := 3

	if params.Cursor != "" {
		cursorTime, cursorIDStr, err := decodeTimestampCursor(params.Cursor)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		cursorUUID, err := parseUUID(cursorIDStr)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		query += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", argN, argN+1)
		args = append(args, pgtype.Timestamptz{Time: cursorTime, Valid: true}, cursorUUID)
		argN += 2
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", argN)
	args = append(args, limit+1)

	return s.queryAuditLog(ctx, query, args, limit)
}

// ListAuditLogGlobal returns audit log entries across all entities.
func (s *Service) ListAuditLogGlobal(ctx context.Context, params AuditLogGlobalParams) (*AuditLogListResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	query := "SELECT id, entity_type, entity_id, action, field, old_value, new_value, actor_type, actor_id, actor_name, metadata, created_at FROM audit_log WHERE 1=1"
	args := []any{}
	argN := 1

	if params.EntityType != nil {
		query += fmt.Sprintf(" AND entity_type = $%d", argN)
		args = append(args, *params.EntityType)
		argN++
	}
	if params.ActorType != nil {
		query += fmt.Sprintf(" AND actor_type = $%d", argN)
		args = append(args, *params.ActorType)
		argN++
	}
	if params.Cursor != "" {
		cursorTime, cursorIDStr, err := decodeTimestampCursor(params.Cursor)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		cursorUUID, err := parseUUID(cursorIDStr)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		query += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", argN, argN+1)
		args = append(args, pgtype.Timestamptz{Time: cursorTime, Valid: true}, cursorUUID)
		argN += 2
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", argN)
	args = append(args, limit+1)

	return s.queryAuditLog(ctx, query, args, limit)
}

func (s *Service) queryAuditLog(ctx context.Context, query string, args []any, limit int) (*AuditLogListResult, error) {
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit log: %w", err)
	}
	defer rows.Close()

	var entries []AuditLogResponse
	for rows.Next() {
		var (
			id         pgtype.UUID
			entityType string
			entityID   pgtype.UUID
			action     string
			field      pgtype.Text
			oldValue   pgtype.Text
			newValue   pgtype.Text
			actorType  string
			actorID    pgtype.Text
			actorName  string
			metadata   []byte
			createdAt  pgtype.Timestamptz
		)
		if err := rows.Scan(&id, &entityType, &entityID, &action, &field, &oldValue, &newValue,
			&actorType, &actorID, &actorName, &metadata, &createdAt); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}

		entry := AuditLogResponse{
			ID:         formatUUID(id),
			EntityType: entityType,
			EntityID:   formatUUID(entityID),
			Action:     action,
			Field:      textPtr(field),
			OldValue:   textPtr(oldValue),
			NewValue:   textPtr(newValue),
			ActorType:  actorType,
			ActorID:    textPtr(actorID),
			ActorName:  actorName,
			CreatedAt:  createdAt.Time.UTC().Format(time.RFC3339),
		}

		if len(metadata) > 0 {
			var m map[string]string
			if json.Unmarshal(metadata, &m) == nil {
				entry.Metadata = m
			}
		}

		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows audit log: %w", err)
	}

	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}

	var nextCursor string
	if hasMore && len(entries) > 0 {
		last := entries[len(entries)-1]
		t, _ := time.Parse(time.RFC3339, last.CreatedAt)
		nextCursor = encodeTimestampCursor(t, last.ID)
	}

	return &AuditLogListResult{
		Entries:    entries,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}
