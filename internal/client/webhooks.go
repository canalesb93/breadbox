package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// WebhookEvent mirrors service.WebhookEventRow.
type WebhookEvent struct {
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

// WebhookEventList is the paginated list response shape.
type WebhookEventList struct {
	WebhookEvents []WebhookEvent `json:"webhook_events"`
	Total         int64          `json:"total"`
	Page          int            `json:"page"`
	PageSize      int            `json:"page_size"`
	TotalPages    int            `json:"total_pages"`
}

// WebhookEventFilters carries the optional filters for ListWebhookEvents.
type WebhookEventFilters struct {
	Provider string
	Status   string
	Page     int
	Limit    int
}

// ListWebhookEvents fetches a page of recent webhook events.
func (c *Client) ListWebhookEvents(ctx context.Context, f WebhookEventFilters) (*WebhookEventList, error) {
	q := url.Values{}
	if f.Provider != "" {
		q.Set("provider", f.Provider)
	}
	if f.Status != "" {
		q.Set("status", f.Status)
	}
	if f.Page > 0 {
		q.Set("page", strconv.Itoa(f.Page))
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	path := "/api/v1/webhook-events"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out WebhookEventList
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WebhookReplayResult is the response shape for /webhook-events/{id}/replay.
type WebhookReplayResult struct {
	WebhookEventID string `json:"webhook_event_id"`
	ConnectionID   string `json:"connection_id,omitempty"`
	Triggered      bool   `json:"triggered"`
	Message        string `json:"message,omitempty"`
}

// ReplayWebhookEvent re-runs the connection sync that the event would have
// triggered.
func (c *Client) ReplayWebhookEvent(ctx context.Context, id string) (*WebhookReplayResult, error) {
	path := "/api/v1/webhook-events/" + url.PathEscape(id) + "/replay"
	var out WebhookReplayResult
	if err := c.Do(ctx, http.MethodPost, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
