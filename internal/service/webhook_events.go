package service

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5/pgtype"
)

// WebhookEventRow is the service-layer representation of a webhook event.
type WebhookEventRow struct {
	ID              string  `json:"id"`
	Provider        string  `json:"provider"`
	EventType       string  `json:"event_type"`
	ConnectionID    *string `json:"connection_id,omitempty"`
	InstitutionName *string `json:"institution_name,omitempty"`
	PayloadHash     string  `json:"raw_payload_hash"`
	Status          string  `json:"status"`
	ErrorMessage    *string `json:"error_message,omitempty"`
	CreatedAt       *string `json:"created_at"`
}

// WebhookEventListParams controls filtering and pagination.
type WebhookEventListParams struct {
	Page     int
	PageSize int
	Provider *string
	Status   *string
}

// WebhookEventListResult is a paginated list of webhook events.
type WebhookEventListResult struct {
	Events     []WebhookEventRow
	Total      int64
	Page       int
	PageSize   int
	TotalPages int
}

// WebhookEventStats holds aggregate counts for the dashboard header.
type WebhookEventStats struct {
	TotalEvents    int64
	ReceivedCount  int64
	ProcessedCount int64
	ErrorCount     int64
}

// ListWebhookEventsPaginated returns a filtered, paginated list of webhook events.
func (s *Service) ListWebhookEventsPaginated(ctx context.Context, params WebhookEventListParams) (*WebhookEventListResult, error) {
	if params.PageSize <= 0 {
		params.PageSize = 25
	}
	if params.Page <= 0 {
		params.Page = 1
	}

	// Build the filtered query.
	baseWhere := "WHERE 1=1"
	args := []any{}
	argN := 1

	if params.Provider != nil && *params.Provider != "" {
		baseWhere += fmt.Sprintf(" AND we.provider = $%d::provider_type", argN)
		args = append(args, *params.Provider)
		argN++
	}
	if params.Status != nil && *params.Status != "" {
		baseWhere += fmt.Sprintf(" AND we.status = $%d", argN)
		args = append(args, *params.Status)
		argN++
	}

	// Count query.
	countQuery := "SELECT COUNT(*) FROM webhook_events we " + baseWhere
	var total int64
	if err := s.Pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count webhook events: %w", err)
	}

	// Data query with LEFT JOIN to bank_connections for institution name.
	dataQuery := `SELECT we.id, we.provider, we.event_type, we.connection_id,
		bc.institution_name, we.raw_payload_hash, we.status, we.error_message, we.created_at
		FROM webhook_events we
		LEFT JOIN bank_connections bc ON we.connection_id = bc.id ` +
		baseWhere +
		fmt.Sprintf(" ORDER BY we.created_at DESC LIMIT $%d OFFSET $%d", argN, argN+1)
	args = append(args, params.PageSize, (params.Page-1)*params.PageSize)

	rows, err := s.Pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query webhook events: %w", err)
	}
	defer rows.Close()

	var events []WebhookEventRow
	for rows.Next() {
		var (
			id              pgtype.UUID
			provider        string
			eventType       string
			connectionID    pgtype.UUID
			institutionName pgtype.Text
			payloadHash     string
			status          string
			errorMessage    pgtype.Text
			createdAt       pgtype.Timestamptz
		)
		if err := rows.Scan(&id, &provider, &eventType, &connectionID,
			&institutionName, &payloadHash, &status, &errorMessage, &createdAt); err != nil {
			return nil, fmt.Errorf("scan webhook event: %w", err)
		}
		row := WebhookEventRow{
			ID:          formatUUID(id),
			Provider:    provider,
			EventType:   eventType,
			PayloadHash: payloadHash,
			Status:      status,
			CreatedAt:   timestampStr(createdAt),
		}
		if connectionID.Valid {
			cid := formatUUID(connectionID)
			row.ConnectionID = &cid
		}
		if institutionName.Valid {
			row.InstitutionName = &institutionName.String
		}
		if errorMessage.Valid {
			row.ErrorMessage = &errorMessage.String
		}
		events = append(events, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate webhook events: %w", err)
	}

	totalPages := int(math.Ceil(float64(total) / float64(params.PageSize)))

	return &WebhookEventListResult{
		Events:     events,
		Total:      total,
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalPages: totalPages,
	}, nil
}

// WebhookEventCounts returns aggregate status counts.
func (s *Service) WebhookEventCounts(ctx context.Context) (*WebhookEventStats, error) {
	query := `SELECT
		COUNT(*) AS total,
		COUNT(*) FILTER (WHERE status = 'received') AS received_count,
		COUNT(*) FILTER (WHERE status = 'processed') AS processed_count,
		COUNT(*) FILTER (WHERE status = 'error') AS error_count
	FROM webhook_events`

	var stats WebhookEventStats
	if err := s.Pool.QueryRow(ctx, query).Scan(
		&stats.TotalEvents,
		&stats.ReceivedCount,
		&stats.ProcessedCount,
		&stats.ErrorCount,
	); err != nil {
		return nil, fmt.Errorf("webhook event counts: %w", err)
	}
	return &stats, nil
}

