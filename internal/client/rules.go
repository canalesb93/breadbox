package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// Rule mirrors service.TransactionRuleResponse. The DSL payload is kept
// as a raw json.RawMessage so the CLI can pass user-authored rule files
// through verbatim without re-validating the DSL grammar (server does
// that).
type Rule struct {
	ID            string          `json:"id"`
	ShortID       string          `json:"short_id"`
	Name          string          `json:"name"`
	Conditions    json.RawMessage `json:"conditions,omitempty"`
	Actions       json.RawMessage `json:"actions,omitempty"`
	Trigger       string          `json:"trigger"`
	CategorySlug  *string         `json:"category_slug,omitempty"`
	CategoryName  *string         `json:"category_display_name,omitempty"`
	Priority      int             `json:"priority"`
	Enabled       bool            `json:"enabled"`
	ExpiresAt     *string         `json:"expires_at,omitempty"`
	CreatedByType string          `json:"created_by_type"`
	CreatedByName string          `json:"created_by_name"`
	HitCount      int             `json:"hit_count"`
	LastHitAt     *string         `json:"last_hit_at,omitempty"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}

// RuleListResult mirrors service.TransactionRuleListResult.
type RuleListResult struct {
	Rules      []Rule `json:"rules"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
	Total      int64  `json:"total"`
}

// RuleListParams carries the supported query params for GET /rules.
type RuleListParams struct {
	Enabled      *bool
	CategorySlug string
	Search       string
	Cursor       string
	Limit        int
}

// ListRules fetches a page of rules.
func (c *Client) ListRules(ctx context.Context, p RuleListParams) (*RuleListResult, error) {
	q := url.Values{}
	if p.Enabled != nil {
		q.Set("enabled", strconv.FormatBool(*p.Enabled))
	}
	if p.CategorySlug != "" {
		q.Set("category_slug", p.CategorySlug)
	}
	if p.Search != "" {
		q.Set("search", p.Search)
	}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	path := "/api/v1/rules"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out RuleListResult
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetRule fetches a single rule by id (short_id or uuid).
func (c *Client) GetRule(ctx context.Context, id string) (*Rule, error) {
	var out Rule
	if err := c.Do(ctx, http.MethodGet, "/api/v1/rules/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateRule POSTs the raw DSL JSON body to /api/v1/rules. The CLI does
// only minimal client-side validation; server validates the full grammar.
func (c *Client) CreateRule(ctx context.Context, body json.RawMessage) (*Rule, error) {
	var out Rule
	if err := c.Do(ctx, http.MethodPost, "/api/v1/rules", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateRule PUTs the raw DSL JSON body to /api/v1/rules/{id}.
func (c *Client) UpdateRule(ctx context.Context, id string, body json.RawMessage) (*Rule, error) {
	var out Rule
	if err := c.Do(ctx, http.MethodPut, "/api/v1/rules/"+url.PathEscape(id), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteRule removes a rule by id.
func (c *Client) DeleteRule(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/v1/rules/"+url.PathEscape(id), nil, nil)
}

// ApplyRuleResult is the response payload from POST /api/v1/rules/{id}/apply.
type ApplyRuleResult struct {
	RuleID        string `json:"rule_id"`
	AffectedCount int64  `json:"affected_count"`
}

// ApplyRule applies a rule retroactively.
func (c *Client) ApplyRule(ctx context.Context, id string) (*ApplyRuleResult, error) {
	var out ApplyRuleResult
	if err := c.Do(ctx, http.MethodPost, "/api/v1/rules/"+url.PathEscape(id)+"/apply", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PreviewRuleRequest mirrors the POST /api/v1/rules/preview body.
type PreviewRuleRequest struct {
	Conditions json.RawMessage `json:"conditions"`
	SampleSize int             `json:"sample_size,omitempty"`
}

// PreviewRule fetches matching transactions for the given conditions.
// The response shape is dynamic (service.RulePreviewResult); we surface
// it as a generic map so the CLI can render it without taking a hard
// dependency on the server-side type.
func (c *Client) PreviewRule(ctx context.Context, req PreviewRuleRequest) (map[string]any, error) {
	out := map[string]any{}
	if err := c.Do(ctx, http.MethodPost, "/api/v1/rules/preview", req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// BatchCreateRules sends an array of rule definitions to /rules/batch.
// The server validates each entry; per-op errors live in the response's
// `results[]`.
func (c *Client) BatchCreateRules(ctx context.Context, body json.RawMessage) (map[string]any, error) {
	out := map[string]any{}
	if err := c.Do(ctx, http.MethodPost, "/api/v1/rules/batch", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}
