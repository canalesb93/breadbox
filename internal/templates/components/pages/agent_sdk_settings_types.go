//go:build !headless && !lite

package pages

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
