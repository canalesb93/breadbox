//go:build !headless && !lite

package admin

import (
	"net/http"
	"strconv"
	"sync"

	"breadbox/internal/app"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// AgentSDKSettingsPageHandler serves GET /settings/workflows (or wherever
// router.go wires it) — the v1 admin "Workflows settings" tab for the
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

		// Status + rolling spend fetched concurrently — each is best-effort
		// (display only), so a single query hiccup degrades the relevant
		// element rather than failing the page.
		var (
			status *service.AgentSubsystemStatus
			spend  service.HouseholdSpendStatus
			wg     sync.WaitGroup
		)
		wg.Add(2)
		go func() { defer wg.Done(); status = svc.GetAgentSubsystemStatus(ctx) }()
		go func() { defer wg.Done(); spend, _ = svc.HouseholdSpendStatus(ctx) }()
		wg.Wait()

		form := pages.AgentSDKSettingsFormFields{
			AuthMode:              settings.AuthMode,
			MaxConcurrent:         settings.MaxConcurrent,
			RuntimePath:           settings.RuntimePath,
			TranscriptDir:         settings.TranscriptDir,
			GlobalMaxBudgetUSDStr: formatOptionalBudget(settings.GlobalMaxBudgetUSD),
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

		// Connector library — best-effort (display only).
		connectors, _ := svc.ListConnectors(ctx)
		connectorViews := make([]pages.AgentSDKConnector, 0, len(connectors))
		for _, c := range connectors {
			connectorViews = append(connectorViews, pages.AgentSDKConnector{
				ShortID:    c.ShortID,
				Name:       c.Name,
				URL:        c.URL,
				HeaderName: c.HeaderName,
				HasSecret:  c.HasSecret,
			})
		}

		props := pages.AgentSDKSettingsProps{
			Form:                 form,
			FieldErrors:          map[string]string{},
			FormError:            formError,
			FormSuccess:          formSuccess,
			Status:               buildAgentSDKStatus(status),
			CSRFToken:            GetCSRFToken(r),
			HouseholdSpend30dStr: formatHouseholdSpend(spend),
			Connectors:           connectorViews,
		}

		data := BaseTemplateData(r, sm, "workflows-settings", "Workflows settings")
		data["CSRFToken"] = props.CSRFToken
		data["Flash"] = nil
		renderSettingsTab(tr, w, r, data, pages.SettingsTabWorkflows, pages.AgentSDKSettings(props))
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
