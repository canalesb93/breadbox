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

	// SimpleFIN state. SimpleFIN has no server-level credential — a single
	// pasted setup token is claimed for an access URL that spans every bank the
	// user links at their bridge. The drawer manages that one bridge connection
	// (connect / rotate token) rather than offering a per-bank credential.
	SimpleFINEnabled bool
	SimpleFINFromEnv bool
	// SimpleFINConnected is true when an active SimpleFIN bridge connection
	// already exists; the drawer then shows status + a token-rotation form
	// instead of the first-time connect form.
	SimpleFINConnected   bool
	SimpleFINInstitution string // institution label of the bridge connection
	SimpleFINAccounts    int64  // number of accounts under the bridge connection
	SimpleFINConnShortID string // short_id for the "view connection" link
	// SimpleFINUsers is the household-member list for the first-time connect
	// form's owner <select>. Reuses the connect-wizard's flat user shape.
	SimpleFINUsers []ConnectionNewUser

	// Encryption-key availability (needed to store cert PEM bytes).
	HasEncryptionKey bool

	// Sync interval used by the webhook fallback sentence.
	SyncIntervalMinutes int

	// Per-provider health summaries (always populated for "plaid",
	// "teller", "csv" — handler ensures stub entries exist).
	ProviderHealth map[string]*service.ProviderHealthSummary
}
