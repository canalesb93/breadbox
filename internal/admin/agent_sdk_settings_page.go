//go:build !headless && !lite

package admin

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// AgentSDKSettingsPageHandler serves GET /settings/agents (or wherever
// router.go wires it) — the v1 admin "Agents settings" tab for the
// Claude Agent SDK runner. Distinct from the existing MCP settings page
// at /settings/mcp (which lives in agents_settings.templ and configures
// MCP server instructions / tool toggles).
//
// The page surfaces:
//   - Auth mode (subscription token vs Anthropic API key) with masked credential inputs.
//   - Sidecar binary path + transcript-storage overrides.
//   - Per-run execution ceilings (max concurrent, global max budget).
//   - A diagnostics panel that fires POST /-/agents/test and POST /-/agents/cleanup.
//
// Plaintext credentials never leave the server — only the masked
// display string surfaces as a placeholder. Submitting an empty token
// field is the "keep current value" signal.
func AgentSDKSettingsPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		settings, err := svc.GetAgentSettings(ctx, a.Config.EncryptionKey, a.Config.DataDir)
		if err != nil {
			http.Error(w, "Failed to load agent settings: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Read-only overview inputs fetched concurrently — each is
		// best-effort (display only), so a single query hiccup degrades the
		// relevant tile rather than failing the page.
		var (
			status *service.AgentSubsystemStatus
			spend  service.HouseholdSpendStatus
			defs   []service.AgentDefinitionResponse
			runs   *service.AgentRunListWithAgentResult
			wg     sync.WaitGroup
		)
		wg.Add(4)
		go func() { defer wg.Done(); status = svc.GetAgentSubsystemStatus(ctx) }()
		go func() { defer wg.Done(); spend, _ = svc.HouseholdSpendStatus(ctx) }()
		go func() { defer wg.Done(); defs, _ = svc.ListAgentDefinitions(ctx) }()
		go func() {
			defer wg.Done()
			// Trailing-7-day, preset-backed runs only. Bounded limit keeps the
			// read cheap; 7-day volume sits well under this in practice.
			start := time.Now().Add(-7 * 24 * time.Hour)
			runs, _ = svc.ListAllAgentRuns(ctx, service.AllAgentRunListParams{
				Limit:         200,
				WorkflowsOnly: true,
				Start:         &start,
			})
		}()
		wg.Wait()

		form := pages.AgentSDKSettingsFormFields{
			AuthMode:              settings.AuthMode,
			MaxConcurrent:         settings.MaxConcurrent,
			RuntimePath:           settings.RuntimePath,
			TranscriptDir:         settings.TranscriptDir,
			GlobalMaxBudgetUSDStr: formatOptionalBudget(settings.GlobalMaxBudgetUSD),
			NotifyWebhookURL:      settings.NotifyWebhookURL,
		}
		if settings.SubscriptionToken != nil {
			form.SubscriptionTokenDisplay = *settings.SubscriptionToken
		}
		if settings.AnthropicAPIKey != nil {
			form.AnthropicAPIKeyDisplay = *settings.AnthropicAPIKey
		}

		flash := GetFlash(ctx, sm)
		var formError, formSuccess string
		if flash != nil {
			switch flash.Type {
			case "error":
				formError = flash.Message
			case "success":
				formSuccess = flash.Message
			}
		}

		props := pages.AgentSDKSettingsProps{
			Form:                 form,
			FieldErrors:          map[string]string{},
			FormError:            formError,
			FormSuccess:          formSuccess,
			Status:               buildAgentSDKStatus(status),
			CSRFToken:            GetCSRFToken(r),
			HouseholdSpend30dStr: formatHouseholdSpend(spend),
			Overview:             buildAgentSDKOverview(spend, defs, runs),
		}

		data := BaseTemplateData(r, sm, "agents-settings", "Workflows settings")
		data["CSRFToken"] = props.CSRFToken
		data["Flash"] = nil
		renderSettingsTab(tr, w, r, data, pages.SettingsTabAgents, pages.AgentSDKSettings(props))
	}
}

// formatOptionalBudget renders the global-max-budget value for the form
// input. nil → empty (no cap); otherwise a fixed-point representation
// matching the service-layer storage format.
func formatOptionalBudget(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 2, 64)
}

// formatHouseholdSpend renders the rolling-window spend as "$X.XX" or,
// when a ceiling is set, "$X.XX of $Y.YY".
func formatHouseholdSpend(s service.HouseholdSpendStatus) string {
	spent := "$" + strconv.FormatFloat(s.SpentUSD, 'f', 2, 64)
	if s.CeilingUSD != nil && *s.CeilingUSD > 0 {
		return spent + " of $" + strconv.FormatFloat(*s.CeilingUSD, 'f', 2, 64)
	}
	return spent
}

// buildAgentSDKStatus maps the service readiness struct to the page's
// status-panel props. Nil-safe — a nil status (the service method always
// returns a value in practice) renders as "not ready".
func buildAgentSDKStatus(s *service.AgentSubsystemStatus) pages.AgentSDKStatusProps {
	if s == nil {
		return pages.AgentSDKStatusProps{}
	}
	return pages.AgentSDKStatusProps{
		Ready:          s.Ready,
		AuthConfigured: s.AuthConfigured,
		BinaryPresent:  s.BinaryPresent,
		BinaryPath:     s.BinaryPath,
	}
}

// agentSDKRunStatusOrder fixes the display order + labels + daisy tones of
// the 7-day run breakdown so the templ never switches on the raw status.
// Mirrors the canonical run-status enum (see CLAUDE.md "Sync status" +
// the agents subsystem's added "skipped").
var agentSDKRunStatusOrder = []struct {
	Status string
	Label  string
	Tone   string
}{
	{"success", "Succeeded", "success"},
	{"error", "Failed", "error"},
	{"in_progress", "In progress", "info"},
	{"skipped", "Skipped", "neutral"},
}

// buildAgentSDKOverview derives the read-only "Workflows overview" panel
// from already-fetched data. Everything is best-effort: a nil/empty input
// degrades the relevant tile rather than erroring.
//
//   - EnabledCount counts enabled preset-backed workflows (source_template
//     set) so it lines up with the gallery's notion of a "workflow".
//   - Spend mirrors HouseholdSpendStatus (the rolling 30-day window) and
//     reuses its ceiling for the percent-of-cap hint.
//   - The 7-day run breakdown is bucketed from the bounded run list.
//   - NextRun is the soonest NextFireAt across enabled workflows; the
//     service already computes NextFireAt (quiet-hours-aware) per definition.
func buildAgentSDKOverview(
	spend service.HouseholdSpendStatus,
	defs []service.AgentDefinitionResponse,
	runs *service.AgentRunListWithAgentResult,
) pages.AgentSDKOverviewProps {
	now := time.Now()
	out := pages.AgentSDKOverviewProps{}

	// Enabled workflow count + soonest next-fire.
	var nextAt time.Time
	var nextName string
	for i := range defs {
		d := defs[i]
		if !d.Enabled || d.SourceTemplate == nil {
			continue
		}
		out.EnabledCount++
		if d.NextFireAt == nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, *d.NextFireAt)
		if err != nil || !t.After(now) {
			continue
		}
		if nextAt.IsZero() || t.Before(nextAt) {
			nextAt = t
			nextName = d.Name
		}
	}
	if !nextAt.IsZero() {
		out.HasNextRun = true
		out.NextRunWhenStr = nextAt.Local().Format("Jan 2, 15:04")
		out.NextRunRelStr = agentSDKUntil(nextAt.Sub(now))
		out.NextRunWorkflow = nextName
	}

	// Spend tile.
	out.Spend30dStr = "$" + strconv.FormatFloat(spend.SpentUSD, 'f', 2, 64)
	if spend.CeilingUSD != nil && *spend.CeilingUSD > 0 {
		ceiling := *spend.CeilingUSD
		out.SpendVsCeilingStr = "of $" + strconv.FormatFloat(ceiling, 'f', 2, 64) + " ceiling"
		pct := int(spend.SpentUSD / ceiling * 100)
		if pct < 0 {
			pct = 0
		}
		out.SpendPctStr = strconv.Itoa(pct) + "%"
		out.SpendOverCeiling = spend.SpentUSD >= ceiling
	} else {
		out.SpendVsCeilingStr = "no ceiling set"
	}

	// 7-day run breakdown, in fixed display order.
	counts := map[string]int{}
	if runs != nil {
		for i := range runs.Runs {
			counts[runs.Runs[i].Status]++
			out.Runs7dTotal++
		}
	}
	for _, sc := range agentSDKRunStatusOrder {
		out.Runs7dByStatus = append(out.Runs7dByStatus, pages.AgentSDKRunStatusCount{
			Status: sc.Status,
			Label:  sc.Label,
			Count:  counts[sc.Status],
			Tone:   sc.Tone,
		})
	}

	return out
}

// agentSDKUntil renders a short, forward-looking duration hint ("in 2h",
// "in 3d") for the next-run row. Sub-minute resolves to "imminently".
func agentSDKUntil(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "imminently"
	case d < time.Hour:
		return "in " + strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return "in " + strconv.Itoa(int(d.Hours())) + "h"
	default:
		return "in " + strconv.Itoa(int(d.Hours()/24)) + "d"
	}
}
