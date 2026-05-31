//go:build !headless && !lite

package pages

import (
	"context"
	"strings"
	"testing"
)

// T20WorkflowRunsRenderFixture builds a minimal WorkflowRunsProps with no run
// rows so WorkflowRuns(p) exercises the status TabBar and the empty state.
// Counts are intentionally non-zero so the badge-suppression branch
// (workflowCountPtr) can be exercised; zero-count tabs yield no badge.
func T20WorkflowRunsRenderFixture() WorkflowRunsProps {
	return WorkflowRunsProps{
		Rows:           nil,
		StatusFilter:   "",
		WorkflowSlug:   "",
		Options:        nil,
		Limit:          50,
		Offset:         0,
		HasMore:        false,
		CSRFToken:      "csrf-test-token",
		SubsystemReady: true,
		Counts: WorkflowRunStatusCounts{
			All:        12,
			Success:    8,
			Error:      2,
			InProgress: 1,
			Skipped:    1,
		},
	}
}

// TestT20WorkflowRunsStatusTabLabels asserts that all five status tabs render
// with the expected human-readable labels: "All", "Success", "Error",
// "In progress", and "Skipped". The TabBar is emitted by workflowRunsFilters
// inside WorkflowRuns, so a single top-level render covers the whole path.
func TestT20WorkflowRunsStatusTabLabels(t *testing.T) {
	p := T20WorkflowRunsRenderFixture()

	var buf strings.Builder
	if err := WorkflowRuns(p).Render(context.Background(), &buf); err != nil {
		t.Fatalf("WorkflowRuns render returned error: %v", err)
	}
	html := buf.String()

	wantLabels := []string{"All", "Success", "Error", "In progress", "Skipped"}
	for _, label := range wantLabels {
		if !strings.Contains(html, label) {
			t.Errorf("expected rendered HTML to contain tab label %q\nrendered (%d bytes):\n%s",
				label, buf.Len(), html)
		}
	}
}

// TestT20WorkflowRunsStatusTabBarPresent asserts that the TabBar wrapper
// element (role="tablist") with the correct aria-label is present in the
// rendered output, confirming that workflowRunsFilters is called and
// emits the daisy tablist shape.
func TestT20WorkflowRunsStatusTabBarPresent(t *testing.T) {
	p := T20WorkflowRunsRenderFixture()

	var buf strings.Builder
	if err := WorkflowRuns(p).Render(context.Background(), &buf); err != nil {
		t.Fatalf("WorkflowRuns render returned error: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, `role="tablist"`) {
		t.Error(`expected rendered HTML to contain role="tablist" from TabBar`)
	}
	if !strings.Contains(html, "Filter runs by status") {
		t.Error(`expected rendered HTML to contain aria-label "Filter runs by status"`)
	}
}

// TestT20WorkflowRunsEmptyState_NoFilter asserts that when there are no run
// rows and no active status/workflow filter, the empty state renders with the
// "no runs yet" headline. The workflowRunsEmpty helper uses EmptyState with
// Compact=true and InCard=true, so only the Title field is rendered (the
// compact body variant omits Body copy per the EmptyState design contract).
func TestT20WorkflowRunsEmptyState_NoFilter(t *testing.T) {
	p := T20WorkflowRunsRenderFixture()
	// p.StatusFilter == "" and p.WorkflowSlug == "" → "No workflow runs yet"

	var buf strings.Builder
	if err := WorkflowRuns(p).Render(context.Background(), &buf); err != nil {
		t.Fatalf("WorkflowRuns render returned error: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "No workflow runs yet") {
		t.Errorf("empty state (no filter): expected HTML to contain %q\nrendered (%d bytes):\n%s",
			"No workflow runs yet", buf.Len(), html)
	}

	// The filtered headline must NOT appear when no filter is active.
	if strings.Contains(html, "No runs match this filter") {
		t.Error(`empty state (no filter): unexpected filtered headline "No runs match this filter"`)
	}
}

// TestT20WorkflowRunsEmptyState_WithStatusFilter asserts that activating a
// status filter changes the empty-state headline to the "no match" variant.
// The body copy ("Runs appear here…") is passed to EmptyState.Body but is
// intentionally not rendered in the compact variant (Compact=true, InCard=true).
func TestT20WorkflowRunsEmptyState_WithStatusFilter(t *testing.T) {
	p := T20WorkflowRunsRenderFixture()
	p.StatusFilter = "error" // trigger the filtered-empty branch

	var buf strings.Builder
	if err := WorkflowRuns(p).Render(context.Background(), &buf); err != nil {
		t.Fatalf("WorkflowRuns (status filter) render returned error: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "No runs match this filter") {
		t.Errorf("empty state (status=error): expected HTML to contain %q\nrendered (%d bytes):\n%s",
			"No runs match this filter", buf.Len(), html)
	}

	// The no-filter headline must NOT appear when a filter is active.
	if strings.Contains(html, "No workflow runs yet") {
		t.Error(`empty state (status=error): unexpected no-filter headline "No workflow runs yet"`)
	}
}

// TestT20WorkflowRunsEmptyState_WithWorkflowFilter mirrors the status-filter
// case but exercises the workflow-slug branch of workflowRunsEmptyTitle.
func TestT20WorkflowRunsEmptyState_WithWorkflowFilter(t *testing.T) {
	p := T20WorkflowRunsRenderFixture()
	p.WorkflowSlug = "weekly-review" // trigger the filtered-empty branch

	var buf strings.Builder
	if err := WorkflowRuns(p).Render(context.Background(), &buf); err != nil {
		t.Fatalf("WorkflowRuns (workflow filter) render returned error: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "No runs match this filter") {
		t.Errorf(`empty state (workflow filter): expected "No runs match this filter"`+"\nrendered (%d bytes):\n%s",
			buf.Len(), html)
	}
	if strings.Contains(html, "No workflow runs yet") {
		t.Error(`empty state (workflow filter): unexpected no-filter headline "No workflow runs yet"`)
	}
}

// TestT20WorkflowRunsTabLinksPresent asserts that every status tab emits an
// anchor href pointing at /workflows/runs with the expected query-string shape.
// This confirms workflowRunsHref is wired correctly and no tab links to a
// stale route.
func TestT20WorkflowRunsTabLinksPresent(t *testing.T) {
	p := T20WorkflowRunsRenderFixture()

	var buf strings.Builder
	if err := WorkflowRuns(p).Render(context.Background(), &buf); err != nil {
		t.Fatalf("WorkflowRuns render returned error: %v", err)
	}
	html := buf.String()

	wantHrefs := []string{
		`href="/workflows/runs"`,
		`href="/workflows/runs?status=success"`,
		`href="/workflows/runs?status=error"`,
		`href="/workflows/runs?status=in_progress"`,
		`href="/workflows/runs?status=skipped"`,
	}
	for _, href := range wantHrefs {
		if !strings.Contains(html, href) {
			t.Errorf("expected rendered HTML to contain tab link %q\nrendered (%d bytes):\n%s",
				href, buf.Len(), html)
		}
	}
}

// TestT20WorkflowCountPtr_ZeroCollapsesToNil guards the badge-suppression
// helper: a count of zero must return nil (no badge rendered) whereas a
// positive count must return a non-nil pointer with the correct value.
func TestT20WorkflowCountPtr_ZeroCollapsesToNil(t *testing.T) {
	if got := workflowCountPtr(0); got != nil {
		t.Errorf("workflowCountPtr(0): expected nil, got %v", got)
	}
	if got := workflowCountPtr(5); got == nil {
		t.Error("workflowCountPtr(5): expected non-nil pointer")
	} else if *got != 5 {
		t.Errorf("workflowCountPtr(5): expected *5, got *%d", *got)
	}
}

// TestT20WorkflowRunsEmptyTitleFunc exercises workflowRunsEmptyTitle directly
// to ensure the filter-presence logic is correct in isolation.
func TestT20WorkflowRunsEmptyTitleFunc(t *testing.T) {
	cases := []struct {
		name   string
		props  WorkflowRunsProps
		wantSz string
	}{
		{
			name:   "no_filter",
			props:  WorkflowRunsProps{},
			wantSz: "No workflow runs yet",
		},
		{
			name:   "status_filter",
			props:  WorkflowRunsProps{StatusFilter: "success"},
			wantSz: "No runs match this filter",
		},
		{
			name:   "workflow_slug_filter",
			props:  WorkflowRunsProps{WorkflowSlug: "some-workflow"},
			wantSz: "No runs match this filter",
		},
		{
			name:   "both_filters",
			props:  WorkflowRunsProps{StatusFilter: "error", WorkflowSlug: "some-workflow"},
			wantSz: "No runs match this filter",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := workflowRunsEmptyTitle(tc.props)
			if got != tc.wantSz {
				t.Errorf("workflowRunsEmptyTitle(%+v) = %q, want %q", tc.props, got, tc.wantSz)
			}
		})
	}
}
