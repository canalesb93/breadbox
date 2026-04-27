package service

import (
	"context"
	"fmt"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
)

// AgentReportResponse is the API response type for agent reports.
type AgentReportResponse struct {
	ID            string   `json:"id"`
	ShortID       string   `json:"short_id"`
	Title         string   `json:"title"`
	Body          string   `json:"body"`
	CreatedByType string   `json:"created_by_type"`
	CreatedByID   *string  `json:"created_by_id"`
	CreatedByName string   `json:"created_by_name"`
	Priority      string   `json:"priority"`
	Tags          []string `json:"tags"`
	Author        *string  `json:"author,omitempty"`
	ReadAt        *string  `json:"read_at"`
	CreatedAt     string   `json:"created_at"`
}

func agentReportFromRow(r db.AgentReport) AgentReportResponse {
	tags := r.Tags
	if tags == nil {
		tags = []string{}
	}
	return AgentReportResponse{
		ID:            formatUUID(r.ID),
		ShortID:       r.ShortID,
		Title:         r.Title,
		Body:          r.Body,
		CreatedByType: r.CreatedByType,
		CreatedByID:   textPtr(r.CreatedByID),
		CreatedByName: r.CreatedByName,
		Priority:      r.Priority,
		Tags:          tags,
		Author:        textPtr(r.Author),
		ReadAt:        timestampStr(r.ReadAt),
		CreatedAt:     pgconv.TimestampStr(r.CreatedAt),
	}
}

// ValidReportPriorities lists allowed priority values.
var ValidReportPriorities = map[string]bool{
	"info":     true,
	"warning":  true,
	"critical": true,
}

// CreateAgentReport creates a new agent report, optionally linked to an MCP session.
func (s *Service) CreateAgentReport(ctx context.Context, title, body string, actor Actor, priority string, tags []string, author string, sessionID string) (AgentReportResponse, error) {
	if title == "" {
		return AgentReportResponse{}, fmt.Errorf("%w: title is required", ErrInvalidParameter)
	}
	if body == "" {
		return AgentReportResponse{}, fmt.Errorf("%w: body is required", ErrInvalidParameter)
	}
	if priority == "" {
		priority = "info"
	}
	if !ValidReportPriorities[priority] {
		return AgentReportResponse{}, fmt.Errorf("%w: priority must be info, warning, or critical", ErrInvalidParameter)
	}
	if tags == nil {
		tags = []string{}
	}
	if len(tags) > 10 {
		return AgentReportResponse{}, fmt.Errorf("%w: maximum 10 tags allowed", ErrInvalidParameter)
	}

	createdByName := actor.Name
	if author != "" {
		createdByName = author
	}

	sessUUID, _ := s.ResolveSessionUUID(ctx, sessionID)

	report, err := s.Queries.CreateAgentReport(ctx, db.CreateAgentReportParams{
		Title:         title,
		Body:          body,
		CreatedByType: actor.Type,
		CreatedByID:   pgconv.TextIfNotEmpty(actor.ID),
		CreatedByName: createdByName,
		Priority:      priority,
		Tags:          tags,
		Author:        pgconv.TextIfNotEmpty(author),
		SessionID:     sessUUID,
	})
	if err != nil {
		return AgentReportResponse{}, fmt.Errorf("create agent report: %w", err)
	}

	return agentReportFromRow(report), nil
}

// ListAgentReports returns the most recent reports.
func (s *Service) ListAgentReports(ctx context.Context, limit int) ([]AgentReportResponse, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.Queries.ListAgentReports(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("list agent reports: %w", err)
	}
	result := make([]AgentReportResponse, len(rows))
	for i, r := range rows {
		result[i] = agentReportFromRow(r)
	}
	return result, nil
}

// ListUnreadAgentReports returns unread reports.
func (s *Service) ListUnreadAgentReports(ctx context.Context, limit int) ([]AgentReportResponse, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	rows, err := s.Queries.ListUnreadAgentReports(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("list unread agent reports: %w", err)
	}
	result := make([]AgentReportResponse, len(rows))
	for i, r := range rows {
		result[i] = agentReportFromRow(r)
	}
	return result, nil
}

// CountUnreadAgentReports returns the count of unread reports.
func (s *Service) CountUnreadAgentReports(ctx context.Context) (int64, error) {
	return s.Queries.CountUnreadAgentReports(ctx)
}

// GetAgentReport returns a single report by ID.
func (s *Service) GetAgentReport(ctx context.Context, reportID string) (AgentReportResponse, error) {
	uid, err := pgconv.ParseUUID(reportID)
	if err != nil {
		return AgentReportResponse{}, fmt.Errorf("%w: invalid report ID", ErrInvalidParameter)
	}
	row, err := s.Queries.GetAgentReport(ctx, uid)
	if err != nil {
		return AgentReportResponse{}, fmt.Errorf("get agent report: %w", err)
	}
	return agentReportFromRow(row), nil
}

// MarkAgentReportRead marks a single report as read.
func (s *Service) MarkAgentReportRead(ctx context.Context, reportID string) error {
	uid, err := pgconv.ParseUUID(reportID)
	if err != nil {
		return fmt.Errorf("%w: invalid report ID", ErrInvalidParameter)
	}
	return s.Queries.MarkAgentReportRead(ctx, uid)
}

// MarkAgentReportUnread clears read_at on a single report, returning it to the unread queue.
func (s *Service) MarkAgentReportUnread(ctx context.Context, reportID string) error {
	uid, err := pgconv.ParseUUID(reportID)
	if err != nil {
		return fmt.Errorf("%w: invalid report ID", ErrInvalidParameter)
	}
	return s.Queries.MarkAgentReportUnread(ctx, uid)
}

// MarkAllAgentReportsRead marks all unread reports as read.
func (s *Service) MarkAllAgentReportsRead(ctx context.Context) error {
	return s.Queries.MarkAllAgentReportsRead(ctx)
}
