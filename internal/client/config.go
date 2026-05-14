package client

import (
	"context"
	"net/http"
	"net/url"
)

// ConfigEntry mirrors api.configEntry — one row in the app_config list.
type ConfigEntry struct {
	Key    string  `json:"key"`
	Value  *string `json:"value,omitempty"`
	Masked bool    `json:"masked,omitempty"`
	Source string  `json:"source"`
	Secret bool    `json:"secret,omitempty"`
}

// ListConfig fetches every app_config row. Secret values are masked unless
// reveal=true.
func (c *Client) ListConfig(ctx context.Context, reveal bool) ([]ConfigEntry, error) {
	path := "/api/v1/config"
	if reveal {
		path += "?reveal=true"
	}
	var out []ConfigEntry
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetConfig fetches a single config entry. Missing keys return an entry with
// nil Value + source="default", not an error — matches the server semantics
// of "fall back to env or default when the DB row isn't set".
func (c *Client) GetConfig(ctx context.Context, key string, reveal bool) (*ConfigEntry, error) {
	path := "/api/v1/config/" + url.PathEscape(key)
	if reveal {
		path += "?reveal=true"
	}
	var out ConfigEntry
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// setConfigBody is the JSON body for PUT /config/{key}.
type setConfigBody struct {
	Value string `json:"value"`
}

// SetConfig writes a value to app_config. Returns the saved entry.
func (c *Client) SetConfig(ctx context.Context, key, value string) (*ConfigEntry, error) {
	path := "/api/v1/config/" + url.PathEscape(key)
	var out ConfigEntry
	if err := c.Do(ctx, http.MethodPut, path, setConfigBody{Value: value}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteConfig drops an app_config row (effective value falls back to env or
// the compiled-in default).
func (c *Client) DeleteConfig(ctx context.Context, key string) error {
	path := "/api/v1/config/" + url.PathEscape(key)
	return c.Do(ctx, http.MethodDelete, path, nil, nil)
}
