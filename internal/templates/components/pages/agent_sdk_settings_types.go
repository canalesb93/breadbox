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
	// HouseholdSpend30dStr is the read-only rolling 30-day spend across all
	// workflow runs, formatted for the spend-ceiling section (e.g. "$2.13
	// of $20.00" when a ceiling is set, or "$2.13" with no cap).
	HouseholdSpend30dStr string
	// Overview is the read-only "Workflows overview" panel at the top of the
	// page: enabled count, rolling spend vs ceiling, 7-day run breakdown, and
	// the next scheduled run. Derived entirely server-side by the page
	// handler from existing service methods — display only.
	Overview AgentSDKOverviewProps
}

// AgentSDKOverviewProps drives the read-only "Workflows overview" section.
// Everything here is pre-formatted by the page handler so the templ stays
// free of arithmetic and nil-handling. All fields degrade gracefully: a
// failed underlying query leaves the relevant field empty/zero and the
// section renders the remaining tiles.
type AgentSDKOverviewProps struct {
	// EnabledCount is the number of enabled preset-backed workflows.
	EnabledCount int
	// Spend30dStr is the rolling-window spend, e.g. "$2.13".
	Spend30dStr string
	// SpendVsCeilingStr is the spend tile's caption: "of $20.00 ceiling"
	// when a cap is set, "no ceiling" otherwise.
	SpendVsCeilingStr string
	// SpendPctStr is the percent-of-ceiling string ("11%") or "" when no
	// ceiling is configured. SpendOverCeiling flags >= 100%.
	SpendPctStr      string
	SpendOverCeiling bool
	// Runs7dTotal is the total run count in the trailing 7 days (every
	// status). Runs7dByStatus holds the per-status breakdown in a fixed,
	// display-ordered slice.
	Runs7dTotal    int
	Runs7dByStatus []AgentSDKRunStatusCount
	// HasNextRun is false when nothing is scheduled (no enabled cron
	// workflows). NextRun* describe the soonest upcoming scheduled run
	// across all enabled workflows.
	HasNextRun      bool
	NextRunWhenStr  string // absolute local datetime, e.g. "May 31, 14:00"
	NextRunRelStr   string // "in 2h", "in 3d" — short relative hint
	NextRunWorkflow string // the workflow name that fires next
}

// AgentSDKRunStatusCount is one status bucket in the 7-day run breakdown.
// Tone maps to a daisy badge tone ("success" | "error" | "info" |
// "neutral") so the templ doesn't switch on the raw status string.
type AgentSDKRunStatusCount struct {
	Status string // canonical run status: success | error | in_progress | skipped
	Label  string // display label, e.g. "Succeeded"
	Count  int
	Tone   string // daisy tone: success | error | info | neutral
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
	NotifyWebhookURL         string // outbound notification webhook (empty = off)
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
