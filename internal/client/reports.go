package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Report mirrors service.AgentReportResponse — the JSON shape returned
// by GET /api/v1/reports and friends.
type Report struct {
	ID            string   `json:"id"`
	ShortID       string   `json:"short_id"`
	Title         string   `json:"title"`
	Body          string   `json:"body"`
	CreatedByType string   `json:"created_by_type"`
	CreatedByID   *string  `json:"created_by_id,omitempty"`
	CreatedByName string   `json:"created_by_name"`
	Priority      string   `json:"priority"`
	Tags          []string `json:"tags"`
	Author        *string  `json:"author,omitempty"`
	ReadAt        *string  `json:"read_at,omitempty"`
	CreatedAt     string   `json:"created_at"`
}

// CreateReportParams matches the POST /api/v1/reports body.
type CreateReportParams struct {
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Priority string   `json:"priority,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Author   string   `json:"author,omitempty"`
}

// ListReports returns the most recent reports (server-capped to 50).
func (c *Client) ListReports(ctx context.Context) ([]Report, error) {
	var out []Report
	if err := c.Do(ctx, http.MethodGet, "/api/v1/reports", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetReport fetches a single report.
func (c *Client) GetReport(ctx context.Context, id string) (*Report, error) {
	var out Report
	if err := c.Do(ctx, http.MethodGet, "/api/v1/reports/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateReport submits a new agent report. The CLI lets callers either
// pass a CreateReportParams directly or hand it a raw JSON body — the
// latter is what `breadbox reports submit --json <file>` uses.
func (c *Client) CreateReport(ctx context.Context, body json.RawMessage) (*Report, error) {
	var out Report
	if err := c.Do(ctx, http.MethodPost, "/api/v1/reports", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MarkReportRead flips the read flag.
func (c *Client) MarkReportRead(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodPatch, "/api/v1/reports/"+url.PathEscape(id)+"/read", nil, nil)
}

// MarkReportUnread flips the read flag back.
func (c *Client) MarkReportUnread(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodPatch, "/api/v1/reports/"+url.PathEscape(id)+"/unread", nil, nil)
}
