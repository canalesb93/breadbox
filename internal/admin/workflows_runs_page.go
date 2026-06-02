//go:build !headless && !lite

package admin

import (
	"net/http"
	"strconv"
	"sync"

	"breadbox/internal/service"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// workflowRunsPageLimit is the fixed page size for the Workflows runs
// tab. Offset pagination (prev/next) steps by this amount.
const workflowRunsPageLimit = 50

// validWorkflowRunStatus whitelists the status filter values surfaced as
// tabs; anything else is treated as "all".
var validWorkflowRunStatus = map[string]bool{
	"success":     true,
	"error":       true,
	"in_progress": true,
	"skipped":     true,
}

// WorkflowRunsPageHandler renders GET /workflows/runs — the run history
// scoped to preset-instantiated workflows. Reuses the cross-agent runs
// query with WorkflowsOnly set, the shared AgentRunRow shape, and the
// existing admin run-now endpoint for retrigger. Mirrors the layering
// of AgentRunsListPageHandler but trimmed to the Workflows surface.
func WorkflowRunsPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		status := r.URL.Query().Get("status")
		if !validWorkflowRunStatus[status] {
			status = ""
		}
		workflowSlug := r.URL.Query().Get("workflow")
		offset := 0
		if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v > 0 {
			offset = v
		}

		var (
			presets   []service.WorkflowPresetView
			subStatus *service.AgentSubsystemStatus
			wg        sync.WaitGroup
		)
		wg.Add(2)
		go func() { defer wg.Done(); presets, _ = svc.ListWorkflowPresets(ctx) }()
		go func() { defer wg.Done(); subStatus = svc.GetAgentSubsystemStatus(ctx) }()
		wg.Wait()

		params := service.AllAgentRunListParams{
			Limit:         workflowRunsPageLimit,
			Offset:        offset,
			Status:        status,
			AgentSlugOrID: workflowSlug,
			WorkflowsOnly: true,
		}
		result, err := svc.ListAllAgentRuns(ctx, params)
		if err != nil {
			// Bad workflow filter slug → drop it rather than erroring the
			// whole page (matches the admin "silently drop invalid
			// filters" convention).
			params.AgentSlugOrID = ""
			workflowSlug = ""
			result, err = svc.ListAllAgentRuns(ctx, params)
			if err != nil {
				tr.RenderError(w, r)
				return
			}
		}

		// Status-tab badges respect the (finalized) workflow filter but not
		// the status filter, so each tab shows its own tally. Best-effort.
		counts, _ := svc.WorkflowRunStatusCounts(ctx, workflowSlug)
		all := 0
		for _, n := range counts {
			all += n
		}

		props := pages.WorkflowRunsProps{
			StatusFilter:   status,
			WorkflowSlug:   workflowSlug,
			Options:        workflowRunFilterOptions(presets),
			Limit:          workflowRunsPageLimit,
			Offset:         offset,
			HasMore:        result.HasMore,
			CSRFToken:      GetCSRFToken(r),
			SubsystemReady: subStatus != nil && subStatus.Ready,
			Counts: pages.WorkflowRunStatusCounts{
				All:        all,
				Success:    counts["success"],
				Error:      counts["error"],
				InProgress: counts["in_progress"],
				Skipped:    counts["skipped"],
			},
		}

		props.Rows = make([]components.AgentRunRowProps, 0, len(result.Runs))
		runIDs := make([]string, 0, len(result.Runs))
		for _, run := range result.Runs {
			row := agentRunRowFromResponse(run.AgentRunResponse)
			row.AgentSlug = run.AgentSlug
			row.AgentName = run.AgentName
			row.ShowAgent = true
			props.Rows = append(props.Rows, row)
			runIDs = append(runIDs, run.AgentRunResponse.ID)
		}

		// Batched report chips — best-effort; failure just drops the chips.
		if reportMap, rerr := svc.ListReportSummariesForRunIDs(ctx, runIDs); rerr == nil {
			for i := range props.Rows {
				reps, ok := reportMap[runIDs[i]]
				if !ok {
					continue
				}
				props.Rows[i].Reports = make([]components.AgentRunReportRef, 0, len(reps))
				for _, rep := range reps {
					props.Rows[i].Reports = append(props.Rows[i].Reports, components.AgentRunReportRef{
						ShortID:  rep.ShortID,
						Title:    rep.Title,
						Priority: rep.Priority,
					})
				}
			}
		}

		data := BaseTemplateData(r, sm, "workflows", "Workflows")
		tr.RenderWithTempl(w, r, data, pages.WorkflowRuns(props))
	}
}

// workflowRunFilterOptions builds the workflow dropdown from the enabled
// presets — each instantiated workflow's slug + name. Disabled presets
// (never instantiated) can't have runs, so they're omitted.
func workflowRunFilterOptions(presets []service.WorkflowPresetView) []pages.WorkflowRunFilterOption {
	opts := make([]pages.WorkflowRunFilterOption, 0, len(presets))
	for _, v := range presets {
		if !v.Enabled || v.WorkflowSlug == nil {
			continue
		}
		opts = append(opts, pages.WorkflowRunFilterOption{
			Slug: *v.WorkflowSlug,
			Name: v.Name,
		})
	}
	return opts
}
