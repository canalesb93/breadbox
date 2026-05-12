package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// SyncTriggerRequest is the body of POST /sync.
type SyncTriggerRequest struct {
	ConnectionID *string `json:"connection_id,omitempty"`
}

// SyncTriggerResponse is the 202 envelope.
type SyncTriggerResponse struct {
	Status string `json:"status"`
}

// SyncHealth mirrors the GET /sync/health payload.
type SyncHealth struct {
	OverallHealth     string  `json:"overall_health"`
	LastSyncTime      *string `json:"last_sync_time,omitempty"`
	LastSyncStatus    string  `json:"last_sync_status"`
	RecentSyncCount   int64   `json:"recent_sync_count"`
	RecentSuccessRate float64 `json:"recent_success_rate"`
	RecentErrorCount  int64   `json:"recent_error_count"`
	ConnectionErrors  int64   `json:"connection_errors"`
	NextSyncTime      string  `json:"next_sync_time,omitempty"`
}

// SyncLog mirrors one row of /sync/logs.
type SyncLog struct {
	ID                   string  `json:"id"`
	ConnectionID         string  `json:"connection_id"`
	InstitutionName      string  `json:"institution_name"`
	Provider             string  `json:"provider,omitempty"`
	Trigger              string  `json:"trigger"`
	Status               string  `json:"status"`
	AddedCount           int32   `json:"added_count"`
	ModifiedCount        int32   `json:"modified_count"`
	RemovedCount         int32   `json:"removed_count"`
	UnchangedCount       int32   `json:"unchanged_count"`
	ErrorMessage         *string `json:"error_message,omitempty"`
	FriendlyErrorMessage *string `json:"friendly_error_message,omitempty"`
	WarningMessage       *string `json:"warning_message,omitempty"`
	StartedAt            *string `json:"started_at,omitempty"`
	CompletedAt          *string `json:"completed_at,omitempty"`
	Duration             *string `json:"duration,omitempty"`
	DurationMs           *int32  `json:"duration_ms,omitempty"`
	AccountsAffected     int64   `json:"accounts_affected"`
}

// SyncLogsResult is the envelope returned by GET /sync/logs.
type SyncLogsResult struct {
	SyncLogs   []SyncLog `json:"sync_logs"`
	NextCursor *string   `json:"next_cursor"`
	HasMore    bool      `json:"has_more"`
	Limit      int       `json:"limit"`
	Total      int64     `json:"total"`
}

// SyncLogFilters is the filter set for GET /sync/logs.
type SyncLogFilters struct {
	ConnectionID string
	Status       string
	Trigger      string
	From         string // RFC3339
	To           string // RFC3339
}

// TriggerSync hits POST /sync; pass connectionID empty to sync all.
func (c *Client) TriggerSync(ctx context.Context, connectionID string) (*SyncTriggerResponse, error) {
	body := SyncTriggerRequest{}
	if connectionID != "" {
		body.ConnectionID = &connectionID
	}
	var out SyncTriggerResponse
	if err := c.Do(ctx, http.MethodPost, "/api/v1/sync", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SyncStatus returns GET /sync/health.
func (c *Client) SyncStatus(ctx context.Context) (*SyncHealth, error) {
	var out SyncHealth
	if err := c.Do(ctx, http.MethodGet, "/api/v1/sync/health", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListSyncLogs returns a page of sync logs. `cursor` is the opaque value
// from a previous page's NextCursor; pass empty for the first page.
func (c *Client) ListSyncLogs(ctx context.Context, filters SyncLogFilters, cursor string, limit int) (*SyncLogsResult, error) {
	q := url.Values{}
	if filters.ConnectionID != "" {
		q.Set("connection_id", filters.ConnectionID)
	}
	if filters.Status != "" {
		q.Set("status", filters.Status)
	}
	if filters.Trigger != "" {
		q.Set("trigger", filters.Trigger)
	}
	if filters.From != "" {
		q.Set("from", filters.From)
	}
	if filters.To != "" {
		q.Set("to", filters.To)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/api/v1/sync/logs"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out SyncLogsResult
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
