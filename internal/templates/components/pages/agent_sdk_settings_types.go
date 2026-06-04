//go:build !headless && !lite

package pages

import "github.com/a-h/templ"

// AgentSDKSettingsProps drives the v1 admin "Agents settings" page —
// distinct from the existing AgentsSettings ("MCP Settings") page. This
// one is for the Claude Agent SDK runner: authentication credentials,
// sidecar binary path, and execution-limit knobs.
//
// All token-bearing fields show ONLY the masked display string
// (`subscription_token_display`, `anthropic_api_key_display`). The
// plaintext never leaves the server. An empty input on POST means
// "leave the current value untouched"; the user has to type a new value
// to overwrite the stored secret.
type AgentSDKSettingsProps struct {
	Form        AgentSDKSettingsFormFields
	FieldErrors map[string]string
	FormError   string
	FormSuccess string
	Status      AgentSDKStatusProps
	CSRFToken   string
	// HouseholdSpend30dStr is the read-only rolling 30-day spend across all
	// workflow runs, formatted for the spend-ceiling section (e.g. "$2.13
	// of $20.00" when a ceiling is set, or "$2.13" with no cap).
	HouseholdSpend30dStr string
	// Connectors is the global custom-MCP connector library shown in the
	// Connectors section. Secrets are masked to HasSecret.
	Connectors []AgentSDKConnector
}

// AgentSDKConnector is the settings-page view of one library connector. The
// secret is never sent to the browser — HasSecret only signals whether one is
// stored, so the edit form can show a "leave blank to keep" affordance.
type AgentSDKConnector struct {
	ShortID    string
	Name       string
	URL        string
	HeaderName string
	HasSecret  bool
}

// connectorActionURL builds the POST action for a connector mutation. Kept as a
// Go helper (not an inline templ.SafeURL literal) so the admin-routes drift
// test — which only validates literal URLs against GET routes — doesn't flag
// these POST-only form actions.
func connectorActionURL(shortID, verb string) templ.SafeURL {
	return templ.SafeURL("/-/workflows/connectors/" + shortID + "/" + verb)
}

// AgentSDKSettingsFormFields mirrors the writable settings exposed by
// service.UpdateAgentSettings. Token fields carry only a masked display
// string — the real value stays server-side.
type AgentSDKSettingsFormFields struct {
	AuthMode                 string // "subscription" or "api_key"
	SubscriptionTokenDisplay string // masked, e.g. "sk-ant-oat01-…4f8a" (empty if unset)
	AnthropicAPIKeyDisplay   string // masked, e.g. "sk-ant-api03-…1234" (empty if unset)
	MaxConcurrent            int
	GlobalMaxBudgetUSDStr    string // string so empty means "no cap"
	RuntimePath              string
	TranscriptDir            string
}

// AgentSDKStatusProps mirrors service.AgentSubsystemStatus for the
// status panel at the top of the page. BinaryPath is empty when the
// sidecar can't be located.
type AgentSDKStatusProps struct {
	Ready          bool
	AuthConfigured bool
	BinaryPresent  bool
	BinaryPath     string
}
