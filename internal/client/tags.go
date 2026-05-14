package client

import (
	"context"
	"net/http"
	"net/url"
)

// Tag mirrors service.TagResponse — the JSON shape returned by
// GET /api/v1/tags and friends.
type Tag struct {
	ID          string  `json:"id"`
	ShortID     string  `json:"short_id"`
	Slug        string  `json:"slug"`
	DisplayName string  `json:"display_name"`
	Description string  `json:"description"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	Lifecycle   string  `json:"lifecycle"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// tagsEnvelope wraps the server's `{"tags":[...]}` payload.
type tagsEnvelope struct {
	Tags []Tag `json:"tags"`
}

// CreateTagParams matches the POST /api/v1/tags body.
type CreateTagParams struct {
	Slug        string  `json:"slug"`
	DisplayName string  `json:"display_name"`
	Description string  `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
}

// UpdateTagParams matches the PATCH /api/v1/tags/{slug} body. Every
// field is optional; omitted fields stay unchanged on the server.
type UpdateTagParams struct {
	DisplayName *string `json:"display_name,omitempty"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	Lifecycle   *string `json:"lifecycle,omitempty"`
}

// ListTags returns every registered tag.
func (c *Client) ListTags(ctx context.Context) ([]Tag, error) {
	var env tagsEnvelope
	if err := c.Do(ctx, http.MethodGet, "/api/v1/tags", nil, &env); err != nil {
		return nil, err
	}
	return env.Tags, nil
}

// GetTag fetches a tag by slug, uuid, or short_id.
func (c *Client) GetTag(ctx context.Context, slug string) (*Tag, error) {
	var out Tag
	if err := c.Do(ctx, http.MethodGet, "/api/v1/tags/"+url.PathEscape(slug), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateTag creates a new tag.
func (c *Client) CreateTag(ctx context.Context, params CreateTagParams) (*Tag, error) {
	var out Tag
	if err := c.Do(ctx, http.MethodPost, "/api/v1/tags", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateTag patches mutable tag fields. Slug is immutable.
func (c *Client) UpdateTag(ctx context.Context, slug string, params UpdateTagParams) (*Tag, error) {
	var out Tag
	if err := c.Do(ctx, http.MethodPatch, "/api/v1/tags/"+url.PathEscape(slug), params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTag removes a tag record. Cascades to transaction_tags rows.
func (c *Client) DeleteTag(ctx context.Context, slug string) error {
	return c.Do(ctx, http.MethodDelete, "/api/v1/tags/"+url.PathEscape(slug), nil, nil)
}
