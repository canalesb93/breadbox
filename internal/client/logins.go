package client

import (
	"context"
	"net/http"
)

// LoginAccount mirrors service.LoginAccountResponse. SetupToken is non-empty
// only on create + reset-password responses; the list endpoint scrubs it.
type LoginAccount struct {
	ID                  string  `json:"id"`
	UserID              string  `json:"user_id"`
	UserName            string  `json:"user_name"`
	UserEmail           *string `json:"user_email"`
	Username            string  `json:"username"`
	Role                string  `json:"role"`
	HasPassword         bool    `json:"has_password"`
	SetupToken          string  `json:"setup_token,omitempty"`
	SetupTokenExpiresAt *string `json:"setup_token_expires_at,omitempty"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
}

// CreateLoginRequest is the body of POST /api/v1/users/{user_id}/login. The
// server uses username-as-email; the CLI exposes `--email` and we pass it
// through as `username`.
type CreateLoginRequest struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

// UpdateLoginRequest is the body of PATCH /api/v1/users/{user_id}/login/{login_id}.
// Role is the only mutable field the service exposes today.
type UpdateLoginRequest struct {
	Role string `json:"role"`
}

// ListLoginAccounts walks every login on the box (flat top-level endpoint).
func (c *Client) ListLoginAccounts(ctx context.Context) ([]LoginAccount, error) {
	var out []LoginAccount
	if err := c.Do(ctx, http.MethodGet, "/api/v1/login-accounts", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateLoginAccount creates a login on the parent user.
func (c *Client) CreateLoginAccount(ctx context.Context, userID string, req CreateLoginRequest) (*LoginAccount, error) {
	var out LoginAccount
	if err := c.Do(ctx, http.MethodPost, "/api/v1/users/"+userID+"/login", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateLoginAccount patches the role on an existing login. The login is
// resolved server-side under the parent user_id.
func (c *Client) UpdateLoginAccount(ctx context.Context, userID, loginID string, req UpdateLoginRequest) (*LoginAccount, error) {
	var out LoginAccount
	if err := c.Do(ctx, http.MethodPatch, "/api/v1/users/"+userID+"/login/"+loginID, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteLoginAccount removes a login by its own UUID (no parent user_id).
func (c *Client) DeleteLoginAccount(ctx context.Context, loginID string) error {
	return c.Do(ctx, http.MethodDelete, "/api/v1/login-accounts/"+loginID, nil, nil)
}

// ResetLoginPasswordResponse is the shape returned by reset-password.
type ResetLoginPasswordResponse struct {
	SetupToken string `json:"setup_token"`
}

// ResetLoginPassword issues a new setup token for the login. The token is
// the one-time secret the member uses to set their password.
func (c *Client) ResetLoginPassword(ctx context.Context, loginID string) (*ResetLoginPasswordResponse, error) {
	var out ResetLoginPasswordResponse
	if err := c.Do(ctx, http.MethodPost, "/api/v1/login-accounts/"+loginID+"/reset-password", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
