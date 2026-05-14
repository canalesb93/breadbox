package client

import (
	"context"
	"net/http"
)

// User mirrors service.UserResponse for the CLI's user noun. The fields are
// kept loose (pointers / optional strings) so the client doesn't have to
// re-validate the server's response shape.
type User struct {
	ID        string  `json:"id"`
	ShortID   string  `json:"short_id"`
	Name      string  `json:"name"`
	Email     *string `json:"email"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// CreateUserRequest is the body of POST /api/v1/users.
type CreateUserRequest struct {
	Name  string  `json:"name"`
	Email *string `json:"email,omitempty"`
}

// UpdateUserRequest is the body of PATCH /api/v1/users/{id}.
// Pointers distinguish "leave alone" (nil) from "clear" (non-nil empty).
type UpdateUserRequest struct {
	Name  *string `json:"name,omitempty"`
	Email *string `json:"email,omitempty"`
}

func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	var out []User
	if err := c.Do(ctx, http.MethodGet, "/api/v1/users", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetUser(ctx context.Context, id string) (*User, error) {
	var u User
	if err := c.Do(ctx, http.MethodGet, "/api/v1/users/"+id, nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (c *Client) CreateUser(ctx context.Context, req CreateUserRequest) (*User, error) {
	var u User
	if err := c.Do(ctx, http.MethodPost, "/api/v1/users", req, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (c *Client) UpdateUser(ctx context.Context, id string, req UpdateUserRequest) (*User, error) {
	var u User
	if err := c.Do(ctx, http.MethodPatch, "/api/v1/users/"+id, req, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (c *Client) DeleteUser(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/v1/users/"+id, nil, nil)
}
