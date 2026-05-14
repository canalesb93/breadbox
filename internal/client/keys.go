package client

import (
	"context"
	"net/http"
)

// APIKey mirrors service.APIKeyResponse. PlaintextKey is only present on the
// CreateAPIKey response (CreateAPIKeyResult); list/get never return it.
type APIKey struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	KeyPrefix  string  `json:"key_prefix"`
	Scope      string  `json:"scope"`
	ActorType  string  `json:"actor_type"`
	ActorName  *string `json:"actor_name,omitempty"`
	LastUsedAt *string `json:"last_used_at"`
	RevokedAt  *string `json:"revoked_at"`
	CreatedAt  string  `json:"created_at"`
}

// CreateAPIKeyRequest is the body of POST /api/v1/api-keys. The server
// defaults `scope` to full_access and `actor_type` to agent when blank;
// the CLI populates both explicitly for clarity.
type CreateAPIKeyRequest struct {
	Name      string `json:"name"`
	Scope     string `json:"scope,omitempty"`
	ActorType string `json:"actor_type,omitempty"`
	ActorName string `json:"actor_name,omitempty"`
}

// CreateAPIKeyResult is the body returned by POST /api/v1/api-keys — the
// full record plus the one-time plaintext key. The CLI surfaces
// PlaintextKey to the user exactly once.
type CreateAPIKeyResult struct {
	APIKey
	PlaintextKey string `json:"plaintext_key"`
}

func (c *Client) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	var out []APIKey
	if err := c.Do(ctx, http.MethodGet, "/api/v1/api-keys", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateAPIKey(ctx context.Context, req CreateAPIKeyRequest) (*CreateAPIKeyResult, error) {
	var out CreateAPIKeyResult
	if err := c.Do(ctx, http.MethodPost, "/api/v1/api-keys", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RevokeAPIKey(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/v1/api-keys/"+id, nil, nil)
}
