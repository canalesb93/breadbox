package client

import (
	"context"
	"net/http"
	"net/url"
)

// Connection mirrors service.ConnectionResponse — the JSON shape returned
// by GET /api/v1/connections.
type Connection struct {
	ID              string  `json:"id"`
	ShortID         string  `json:"short_id"`
	UserID          *string `json:"user_id,omitempty"`
	UserName        *string `json:"user_name,omitempty"`
	Provider        string  `json:"provider"`
	InstitutionID   *string `json:"institution_id,omitempty"`
	InstitutionName *string `json:"institution_name,omitempty"`
	Status          string  `json:"status"`
	ErrorCode       *string `json:"error_code,omitempty"`
	ErrorMessage    *string `json:"error_message,omitempty"`
	LastSyncedAt    *string `json:"last_synced_at,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// ConnectionDetail mirrors service.ConnectionDetailResponse.
type ConnectionDetail struct {
	Connection
	Paused                      bool   `json:"paused"`
	SyncIntervalOverrideMinutes *int32 `json:"sync_interval_override_minutes,omitempty"`
	ConsecutiveFailures         int32  `json:"consecutive_failures"`
	AccountCount                int    `json:"account_count"`
}

// HostedLinkSession mirrors the wire shape of a hosted_link_sessions row
// returned by GET /connections/link/{id} and the POST endpoints.
type HostedLinkSession struct {
	ID                  string   `json:"id"`
	ShortID             string   `json:"short_id"`
	UserID              string   `json:"user_id"`
	Provider            string   `json:"provider"`
	Action              string   `json:"action"`
	ConnectionID        string   `json:"connection_id,omitempty"`
	SingleUse           bool     `json:"single_use"`
	RedirectURL         string   `json:"redirect_url,omitempty"`
	Label               string   `json:"label,omitempty"`
	Status              string   `json:"status"`
	ErrorCode           string   `json:"error_code,omitempty"`
	ErrorMessage        string   `json:"error_message,omitempty"`
	ResultConnectionIDs []string `json:"result_connection_ids"`
	ExpiresAt           string   `json:"expires_at"`
	StartedAt           *string  `json:"started_at,omitempty"`
	CompletedAt         *string  `json:"completed_at,omitempty"`
	CreatedAt           string   `json:"created_at"`
}

// CreateHostedLinkResponse mirrors the POST /connections/link response —
// the session plus the one-time-only `token` + `url`.
type CreateHostedLinkResponse struct {
	HostedLinkSession
	Token string `json:"token"`
	URL   string `json:"url"`
}

// CreateHostedLinkParams is the body of POST /connections/link.
type CreateHostedLinkParams struct {
	UserID           string `json:"user_id"`
	Provider         string `json:"provider,omitempty"`
	SingleUse        bool   `json:"single_use,omitempty"`
	RedirectURL      string `json:"redirect_url,omitempty"`
	Label            string `json:"label,omitempty"`
	ExpiresInSeconds *int   `json:"expires_in_seconds,omitempty"`
}

// CreateRelinkParams is the body of POST /connections/{id}/relink.
type CreateRelinkParams struct {
	RedirectURL      string `json:"redirect_url,omitempty"`
	Label            string `json:"label,omitempty"`
	ExpiresInSeconds *int   `json:"expires_in_seconds,omitempty"`
}

// ListConnections returns the household's bank connections.
func (c *Client) ListConnections(ctx context.Context, userID string) ([]Connection, error) {
	path := "/api/v1/connections"
	if userID != "" {
		path += "?" + url.Values{"user_id": []string{userID}}.Encode()
	}
	var out []Connection
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetConnection fetches a single connection by short_id or uuid.
func (c *Client) GetConnection(ctx context.Context, id string) (*ConnectionDetail, error) {
	var out ConnectionDetail
	if err := c.Do(ctx, http.MethodGet, "/api/v1/connections/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateHostedLink mints a hosted-link session and returns the URL + token.
func (c *Client) CreateHostedLink(ctx context.Context, params CreateHostedLinkParams) (*CreateHostedLinkResponse, error) {
	var out CreateHostedLinkResponse
	if err := c.Do(ctx, http.MethodPost, "/api/v1/connections/link", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetHostedLinkSession polls the session by id (uuid or short_id).
func (c *Client) GetHostedLinkSession(ctx context.Context, id string) (*HostedLinkSession, error) {
	var out HostedLinkSession
	if err := c.Do(ctx, http.MethodGet, "/api/v1/connections/link/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateRelink mints a re-auth hosted-link session for an existing connection.
func (c *Client) CreateRelink(ctx context.Context, connectionID string, params CreateRelinkParams) (*CreateHostedLinkResponse, error) {
	var out CreateHostedLinkResponse
	path := "/api/v1/connections/" + url.PathEscape(connectionID) + "/relink"
	if err := c.Do(ctx, http.MethodPost, path, params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DisconnectConnection performs the server's soft-disconnect: DELETE
// /connections/{id}. The server keeps the row + history; only credentials
// are wiped and status flips to 'disconnected'.
//
// There is no hard-delete REST endpoint today — `breadbox connections
// delete` aliases to this same call.
func (c *Client) DisconnectConnection(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/v1/connections/"+url.PathEscape(id), nil, nil)
}
