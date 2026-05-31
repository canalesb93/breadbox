//go:build !headless && !lite

package pages

import (
	"net/url"
	"strconv"

	"breadbox/internal/templates/components"

	"github.com/a-h/templ"
)

// WorkflowRunsProps is the view-model for the /workflows/runs tab — the
// run history scoped to preset-instantiated workflows. It reuses the
// shared AgentRunRow shape for each row; the page adds a status TabBar,
// a workflow filter, a per-run retrigger action, and offset pagination.
type WorkflowRunsProps struct {
	Rows         []components.AgentRunRowProps
	StatusFilter string                    // "" | success | error | in_progress | skipped
	WorkflowSlug string                    // active workflow filter ("" = all)
	Options      []WorkflowRunFilterOption // workflow dropdown options (enabled workflows)
	Limit        int
	Offset       int
	HasMore      bool
	CSRFToken    string

	// SubsystemReady gates the retrigger action — when the agent runtime
	// isn't configured, the "Re-run" item renders disabled instead of
	// firing a run that would only fail.
	SubsystemReady bool
}

// WorkflowRunFilterOption is one entry in the workflow filter dropdown.
type WorkflowRunFilterOption struct {
	Slug string
	Name string
}

// workflowRunRow attaches the per-row retrigger OverflowMenu to a base
// run row. Kept as a Go helper (rather than inline templ) so the
// handler can build plain AgentRunRowProps and the templ layer owns the
// action wiring.
func workflowRunRow(r components.AgentRunRowProps, ready bool) components.AgentRunRowProps {
	r.Actions = workflowRunActions(r.AgentSlug, r.ShortID, ready)
	return r
}

// workflowRunsHref builds a /workflows/runs URL preserving the status +
// workflow filters. Used by the status TabBar so switching status keeps
// the workflow filter, and vice versa.
func workflowRunsHref(status, workflow string) templ.SafeURL {
	return templ.SafeURL(workflowRunsURL(status, workflow, 0))
}

// workflowRunsPageHref steps the offset by one page in either direction
// while preserving both filters. dir is -1 (prev) or +1 (next).
func workflowRunsPageHref(p WorkflowRunsProps, dir int) templ.SafeURL {
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	off := p.Offset + dir*limit
	if off < 0 {
		off = 0
	}
	return templ.SafeURL(workflowRunsURL(p.StatusFilter, p.WorkflowSlug, off))
}

func workflowRunsURL(status, workflow string, offset int) string {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	if workflow != "" {
		q.Set("workflow", workflow)
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	s := "/workflows/runs"
	if enc := q.Encode(); enc != "" {
		s += "?" + enc
	}
	return s
}

// workflowRunsEmptyTitle adapts the empty-state headline to whether a
// filter is active, so a filtered-to-nothing view doesn't read as "no
// runs ever".
func workflowRunsEmptyTitle(p WorkflowRunsProps) string {
	if p.StatusFilter != "" || p.WorkflowSlug != "" {
		return "No runs match this filter"
	}
	return "No workflow runs yet"
}
