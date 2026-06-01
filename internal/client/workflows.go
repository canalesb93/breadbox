package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// Workflow mirrors the subset of service.AgentDefinitionResponse the CLI
// renders. Workflows are the instantiated rows behind the gallery presets
// (the table was renamed from agent_definitions). The REST surface still
// lives under /api/v1/workflows. --json passes the decoded struct through,
// so the fields kept here are the ones a self-hoster commonly inspects.
type Workflow struct {
	ID             string  `json:"id"`
	ShortID        string  `json:"short_id"`
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	ToolScope      string  `json:"tool_scope"`
	Model          string  `json:"model"`
	ScheduleCron   *string `json:"schedule_cron,omitempty"`
	Enabled        bool    `json:"enabled"`
	SourceTemplate *string `json:"source_template,omitempty"`
	NextFireAt     *string `json:"next_fire_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

// ListWorkflows fetches every configured workflow. GET /api/v1/workflows
// returns a bare JSON array (no pagination envelope).
func (c *Client) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	var out []Workflow
	if err := c.Do(ctx, http.MethodGet, "/api/v1/workflows", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// WorkflowRun mirrors the per-row shape of GET /api/v1/workflows/runs —
// every run carries its parent workflow's slug + name so the global feed
// can label each row without an extra fetch. The wire keys are still
// agent_slug / agent_name (the table rename didn't touch the run payload).
type WorkflowRun struct {
	ShortID      string   `json:"short_id"`
	WorkflowSlug string   `json:"agent_slug"`
	WorkflowName string   `json:"agent_name"`
	Trigger      string   `json:"trigger"`
	Status       string   `json:"status"`
	StartedAt    string   `json:"started_at"`
	CompletedAt  *string  `json:"completed_at,omitempty"`
	DurationMs   *int     `json:"duration_ms,omitempty"`
	TotalCostUSD *float64 `json:"total_cost_usd,omitempty"`
	TurnCount    *int     `json:"turn_count,omitempty"`
	HitCap       *string  `json:"hit_cap,omitempty"`
	ErrorMessage *string  `json:"error_message,omitempty"`
}

// WorkflowRunListResult is the paginated envelope returned by
// GET /api/v1/workflows/runs (offset-based, not cursor).
type WorkflowRunListResult struct {
	Runs    []WorkflowRun `json:"runs"`
	Limit   int           `json:"limit"`
	Offset  int           `json:"offset"`
	HasMore bool          `json:"has_more"`
}

// WorkflowRunListParams carries the supported query params for the global
// runs feed. Zero values are omitted from the request.
type WorkflowRunListParams struct {
	Workflow string // slug/short_id/uuid filter (sent as ?agent=)
	Status   string
	Trigger  string
	Limit    int
	Offset   int
}

// ListWorkflowRuns fetches a page of the cross-workflow run feed.
func (c *Client) ListWorkflowRuns(ctx context.Context, p WorkflowRunListParams) (*WorkflowRunListResult, error) {
	q := url.Values{}
	if p.Workflow != "" {
		q.Set("agent", p.Workflow)
	}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if p.Trigger != "" {
		q.Set("trigger", p.Trigger)
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Offset > 0 {
		q.Set("offset", strconv.Itoa(p.Offset))
	}
	path := "/api/v1/workflows/runs"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out WorkflowRunListResult
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WorkflowPreset mirrors the gallery template shape from
// GET /api/v1/workflow-presets. The server-side WorkflowPreset struct has
// no JSON tags, so its exported fields serialize with capitalized keys
// (Slug, Name, Category, Icon, Description); the annotation fields added by
// WorkflowPresetView do carry tags. Mirror both exactly.
type WorkflowPreset struct {
	Slug        string `json:"Slug"`
	Name        string `json:"Name"`
	Category    string `json:"Category"`
	Icon        string `json:"Icon"`
	Description string `json:"Description"`
	// Enabled is true when this preset has been instantiated as a workflow.
	Enabled bool `json:"enabled"`
	// WorkflowSlug is the slug of the instantiated workflow (when Enabled).
	WorkflowSlug *string `json:"workflow_slug,omitempty"`
	// WorkflowEnabled is the instantiated workflow's own enabled toggle
	// (a workflow can be instantiated but paused). Nil when not instantiated.
	WorkflowEnabled *bool `json:"workflow_enabled,omitempty"`
}

// ListWorkflowPresets fetches the code-defined gallery catalog annotated
// with per-preset enablement state. GET /api/v1/workflow-presets returns a
// bare JSON array.
func (c *Client) ListWorkflowPresets(ctx context.Context) ([]WorkflowPreset, error) {
	var out []WorkflowPreset
	if err := c.Do(ctx, http.MethodGet, "/api/v1/workflow-presets", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
