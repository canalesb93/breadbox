//go:build !headless && !lite

package pages

import "breadbox/internal/service"

// ProvidersProps mirrors the field set the old providers.html read off
// the layout data map. Built once in admin/providers.go and handed
// directly to the templ component via TemplateRenderer.RenderWithTempl.
type ProvidersProps struct {
	CSRFToken     string
	ConfigSources map[string]string

	// Plaid state
	PlaidConfigured bool
	PlaidFromEnv    bool
	PlaidClientID   string
	PlaidEnv        string
	WebhookURL      string

	// Teller state
	TellerConfigured        bool
	TellerFromEnv           bool
	TellerAppID             string
	TellerEnv               string
	TellerCertFromEnv       bool
	TellerCertConfigured    bool
	TellerWebhookConfigured bool

	// SimpleFIN state. SimpleFIN has no server-level credential — it's an
	// opt-in toggle deciding whether the provider is offered at connect time
	// (the access token is pasted per connection).
	SimpleFINEnabled bool
	SimpleFINFromEnv bool

	// Encryption-key availability (needed to store cert PEM bytes).
	HasEncryptionKey bool

	// Sync interval used by the webhook fallback sentence.
	SyncIntervalMinutes int

	// Per-provider health summaries (always populated for "plaid",
	// "teller", "csv" — handler ensures stub entries exist).
	ProviderHealth map[string]*service.ProviderHealthSummary

	// logo.dev integration (the "API integrations" section) — the
	// counterparty brand-logo toggle and its publishable token, relocated
	// from the General → Counterparties section so all pluggable APIs live on
	// the Providers directory.
	CounterpartyLogos bool
	LogoDevToken      string
}
