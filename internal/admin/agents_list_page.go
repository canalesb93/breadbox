//go:build !headless && !lite

package admin

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// AgentsListPageHandler serves GET /agents — the admin agent-definition
// list. Builds typed templ props by flattening
// service.AgentDefinitionResponse into pages.AgentsListRowProps so the
// templ never reaches into service types directly.
//
// Concurrency: definitions + readiness status are loaded in parallel; the
// status call is cheap (no API egress per agent_settings.go) so we don't
// gate it behind a feature flag.
func AgentsListPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var (
			defs   []service.AgentDefinitionResponse
			defErr error
			status *service.AgentSubsystemStatus
			wg     sync.WaitGroup
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			defs, defErr = svc.ListAgentDefinitions(ctx)
		}()
		go func() {
			defer wg.Done()
			status = svc.GetAgentSubsystemStatus(ctx)
		}()
		wg.Wait()

		if defErr != nil {
			tr.RenderError(w, r)
			return
		}

		rows := make([]pages.AgentsListRowProps, 0, len(defs))
		lastPrefixes := make(map[string]string, len(defs))
		for _, d := range defs {
			rows = append(rows, buildAgentsListRow(d))
			if d.LastPromptPrefix != nil && *d.LastPromptPrefix != "" {
				lastPrefixes[d.Slug] = *d.LastPromptPrefix
			}
		}

		props := pages.AgentsListProps{
			Agents:             rows,
			Status:             buildAgentsListStatus(status),
			LastPromptPrefixes: lastPrefixes,
			CSRFToken:          GetCSRFToken(r),
		}

		data := BaseTemplateData(r, sm, "agents", "Agents")
		tr.RenderWithTempl(w, r, data, pages.AgentsList(props))
	}
}

// buildAgentsListRow flattens one service.AgentDefinitionResponse into the
// templ view-model. Pre-formats relative time + cost rollup so the templ
// stays free of helper imports.
func buildAgentsListRow(d service.AgentDefinitionResponse) pages.AgentsListRowProps {
	row := pages.AgentsListRowProps{
		Slug:                  d.Slug,
		Name:                  d.Name,
		Description:           firstLine(d.Prompt, 120),
		Model:                 d.Model,
		Enabled:               d.Enabled,
		TriggerOnSyncComplete: d.TriggerOnSyncComplete,
	}

	if d.ScheduleCron != nil {
		row.ScheduleCron = *d.ScheduleCron
	}

	if d.LastRun != nil {
		last := &pages.AgentsListLastRunProps{
			ShortID: d.LastRun.ShortID,
			Status:  d.LastRun.Status,
			Trigger: d.LastRun.Trigger,
		}
		// Prefer CompletedAt; fall back to StartedAt for in-progress rows.
		when := d.LastRun.StartedAt
		if d.LastRun.CompletedAt != nil && *d.LastRun.CompletedAt != "" {
			when = *d.LastRun.CompletedAt
		}
		if t, err := time.Parse(time.RFC3339, when); err == nil {
			last.FinishedAt = t
		}
		if d.LastRun.DurationMs != nil {
			last.DurationMs = int64(*d.LastRun.DurationMs)
		}
		if d.LastRun.TotalCostUSD != nil {
			last.CostUSD = *d.LastRun.TotalCostUSD
		}
		row.LastRun = last
	}

	if d.NextFireAt != nil && *d.NextFireAt != "" {
		if t, err := time.Parse(time.RFC3339, *d.NextFireAt); err == nil {
			row.NextRunRelative = relativeUntil(t)
		}
	}

	if d.CostStats30d != nil {
		row.Cost30dUSD = d.CostStats30d.TotalCostUSD
	}

	return row
}

// buildAgentsListStatus maps the service readiness struct to the props
// shape. Nil-safe: a nil status (shouldn't happen in practice — the
// service method always returns a value) renders as "not ready".
func buildAgentsListStatus(s *service.AgentSubsystemStatus) pages.AgentSubsystemStatusProps {
	if s == nil {
		return pages.AgentSubsystemStatusProps{}
	}
	return pages.AgentSubsystemStatusProps{
		Ready:          s.Ready,
		AuthConfigured: s.AuthConfigured,
		BinaryPresent:  s.BinaryPresent,
		BinaryPath:     s.BinaryPath,
	}
}

// firstLine returns the first non-empty line of s, truncated to max
// characters with an ellipsis if it exceeds the cap.
func firstLine(s string, max int) string {
	for _, raw := range strings.Split(s, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if len(line) > max {
			return line[:max] + "…"
		}
		return line
	}
	return ""
}

// relativeUntil renders a future time as "in 2h" / "in 3d". Falls back to
// an absolute date for far-future times. Mirrors the shape of
// admin.relativeTime but for the future direction.
func relativeUntil(t time.Time) string {
	d := time.Until(t)
	if d < 0 {
		return "overdue"
	}
	switch {
	case d < time.Minute:
		return "in <1m"
	case d < time.Hour:
		return formatRelativeUnit(int(d.Minutes()), "m")
	case d < 24*time.Hour:
		return formatRelativeUnit(int(d.Hours()), "h")
	case d < 7*24*time.Hour:
		return formatRelativeUnit(int(d.Hours()/24), "d")
	default:
		return t.Format("Jan 2")
	}
}

func formatRelativeUnit(n int, unit string) string {
	if n <= 0 {
		n = 1
	}
	return fmt.Sprintf("in %d%s", n, unit)
}
