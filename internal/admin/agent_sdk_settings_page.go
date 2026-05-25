//go:build !headless && !lite

package admin

import (
	"net/http"
	"strconv"

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

		settings, err := svc.GetAgentSettings(ctx, a.Config.EncryptionKey)
		if err != nil {
			http.Error(w, "Failed to load agent settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		status := svc.GetAgentSubsystemStatus(ctx)

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

		props := pages.AgentSDKSettingsProps{
			Form:        form,
			FieldErrors: map[string]string{},
			FormError:   formError,
			FormSuccess: formSuccess,
			Status: pages.AgentSDKStatusProps{
				Ready:          status.Ready,
				AuthConfigured: status.AuthConfigured,
				BinaryPresent:  status.BinaryPresent,
				BinaryPath:     status.BinaryPath,
			},
			CSRFToken: GetCSRFToken(r),
		}

		data := BaseTemplateData(r, sm, "agents-settings", "Agents settings")
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
