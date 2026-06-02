//go:build !headless && !lite

package admin

import (
	"errors"
	"net/http"
	"time"

	"breadbox/internal/service"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// agentDetailRecentRunLimit caps the per-agent recent-runs slice on the
// /agents/{slug} landing. The "View all runs" link drops the operator
// into the global /agents?agent=<slug> view for the full history.
const agentDetailRecentRunLimit = 10

// AgentDetailPageHandler serves GET /agents/{slug} — the per-agent
// landing page. Replaces the previous behavior of clicking an agent
// name in the list and jumping straight to the edit form: the edit
// surface stays at /agents/{slug}/edit, reachable from the header's
// Edit button.
//
// The handler fetches three things in parallel: the agent definition
// (for identity + configuration), the lifetime stats rollup, and the
// last N runs. Stats / runs failures degrade gracefully — the page
// still renders the definition.
func AgentDetailPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		slug := chi.URLParam(r, "slug")

		def, err := svc.GetAgentDefinition(ctx, slug)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				tr.RenderNotFound(w, r)
				return
			}
			tr.RenderError(w, r)
			return
		}

		// Lifetime stats + recent runs are best-effort: a failure leaves
		// the page renderable with zero / empty slots.
		stats, _ := svc.GetAgentLifetimeStats(ctx, def.Slug)
		runs, _ := svc.ListAgentRuns(ctx, def.Slug, service.AgentRunListParams{
			Limit: agentDetailRecentRunLimit,
		})

		props := buildAgentDetailProps(def, stats, runs, GetCSRFToken(r))

		data := BaseTemplateData(r, sm, "agents", def.Name)
		data["Breadcrumbs"] = pages.AgentDetailBreadcrumbs(props)
		tr.RenderWithTempl(w, r, data, pages.AgentDetail(props))
	}
}

// buildAgentDetailProps flattens the service responses into the templ's
// typed view-model so the templ never imports service types.
func buildAgentDetailProps(
	def *service.AgentDefinitionResponse,
	stats *service.AgentLifetimeStats,
	runs *service.AgentRunListResult,
	csrfToken string,
) pages.AgentDetailProps {
	p := pages.AgentDetailProps{
		Slug:                  def.Slug,
		Name:                  def.Name,
		Description:           firstLine(def.Prompt, 240),
		Model:                 def.Model,
		Enabled:               def.Enabled,
		TriggerOnSyncComplete: def.TriggerOnSyncComplete,
		ToolScope:             def.ToolScope,
		MaxTurns:              def.MaxTurns,
		AllowedTools:          def.AllowedTools,
		CSRFToken:             csrfToken,
	}

	if def.ScheduleCron != nil {
		p.ScheduleCron = *def.ScheduleCron
	}
	if def.QuietHoursStart != nil {
		p.QuietHoursStart = *def.QuietHoursStart
	}
	if def.QuietHoursEnd != nil {
		p.QuietHoursEnd = *def.QuietHoursEnd
	}
	if def.MaxBudgetUSD != nil {
		p.MaxBudgetUSD = *def.MaxBudgetUSD
		p.HasMaxBudget = true
	}

	if stats != nil {
		p.Stats = pages.AgentDetailStatsProps{
			RunCount:           stats.RunCount,
			SkippedCount:       stats.SkippedCount,
			ErrorCount:         stats.ErrorCount,
			TotalCostUSD:       stats.TotalCostUSD,
			AvgDurationSeconds: stats.AvgDurationSeconds,
		}
	}

	if runs != nil {
		p.Recent = make([]components.AgentRunRowProps, 0, len(runs.Runs))
		for _, run := range runs.Runs {
			row := agentRunRowFromResponse(run)
			// On the per-agent detail page the surrounding context already
			// names the agent — hide the redundant name from each row.
			row.ShowAgent = false
			p.Recent = append(p.Recent, row)
		}
		p.HasMoreRuns = runs.HasMore || len(runs.Runs) >= agentDetailRecentRunLimit
	}

	// Best-effort next-fire-at — only the list endpoint populates
	// NextFireAt, so GetAgentDefinition leaves it nil. We compute a
	// minimal version here from the cron string when available, but
	// leave the slot empty otherwise; the schedule cell still shows
	// the raw cron expression.
	if def.NextFireAt != nil && *def.NextFireAt != "" {
		if t, err := time.Parse(time.RFC3339, *def.NextFireAt); err == nil {
			p.NextRunRelative = relativeUntil(t)
		}
	}

	return p
}
