//go:build !lite

package appconfig

// Agent subsystem config keys. All values live in the app_config table.
// Read via the helpers in read.go (plaintext) or encrypted.go (AES-GCM).
const (
	// KeyAgentAuthMode controls which credential the sidecar uses.
	// Values: "subscription" (Claude OAuth token) or "api_key" (Anthropic API key).
	// Default: "subscription".
	KeyAgentAuthMode = "agent.auth_mode"

	// KeyAgentSubscriptionToken stores the encrypted Claude OAuth token
	// (sk-ant-oat01-…) obtained via `claude setup-token`.
	KeyAgentSubscriptionToken = "agent.subscription_token"

	// KeyAgentAnthropicAPIKey stores the encrypted Anthropic API key (sk-ant-…).
	KeyAgentAnthropicAPIKey = "agent.anthropic_api_key"

	// KeyAgentMaxConcurrent is the server-wide cap on simultaneous agent runs.
	// Default: "1". v1 enforces strictly 1; future iterations may lift the cap.
	KeyAgentMaxConcurrent = "agent.max_concurrent"

	// KeyAgentGlobalMaxBudgetUSD is the absolute ceiling on per-run cost,
	// regardless of per-agent max_budget_usd. Empty = no global ceiling.
	KeyAgentGlobalMaxBudgetUSD = "agent.global_max_budget_usd"

	// KeyAgentRuntimePath is the absolute path to the breadbox-agent binary.
	// When empty, Sidecar.resolveBinary falls through to env BREADBOX_AGENT_BIN,
	// then ./bin/breadbox-agent, then PATH.
	KeyAgentRuntimePath = "agent.runtime_path"

	// KeyAgentTranscriptDir is the directory where agent runs' NDJSON
	// transcripts are written. One file per run, named "<runID>.ndjson".
	// Default: "<data dir>/agent-transcripts" resolved by the runner.
	KeyAgentTranscriptDir = "agent.transcript_dir"

	// KeyAgentRunRetentionDays is the number of days to keep completed
	// agent_runs rows. Default: 30. 0 disables cleanup.
	KeyAgentRunRetentionDays = "agent.run_retention_days"

	// KeyEncryptionKeyAcknowledgedAt is the RFC3339 timestamp at which the
	// admin clicked through the /setup/save-key reveal page. Unset (empty)
	// means the reveal page is still offered; once set, the page redirects
	// away. The key itself remains recoverable from .env or the
	// `breadbox reveal-key` shell command.
	KeyEncryptionKeyAcknowledgedAt = "setup.encryption_key_acknowledged_at"

	// KeyAvatarStyle is the legacy single-style key. New code reads
	// KeyAvatarUserStyle for user avatars and KeyAvatarAgentStyle for
	// agent avatars; this key is retained for back-compat — the
	// startup loader copies it into KeyAvatarUserStyle when the latter
	// is unset so existing deployments keep their configured style.
	//
	// Deprecated: use KeyAvatarUserStyle.
	KeyAvatarStyle = "avatar.dicebear_style"

	// KeyAvatarUserStyle is the DiceBear style slug used for user
	// identicons. Operators pick it from Settings → System.
	// Default: "shapes" (set in internal/avatar).
	KeyAvatarUserStyle = "avatar.dicebear_style_user"

	// KeyAvatarAgentStyle is the DiceBear style slug used for agent
	// identicons. Agents render distinct avatars from users so an
	// AI-authored activity row reads unambiguously against a human
	// one. Default: "bottts-neutral" (robot identicons on a flat
	// transparent background).
	KeyAvatarAgentStyle = "avatar.dicebear_style_agent"

	// KeyWorkflowsConsentAckAt records when the household first
	// acknowledged that enabling a workflow runs Claude over their
	// financial ledger (incurring Anthropic API cost). Stored as an
	// RFC3339 timestamp; non-empty = acknowledged. Gates the consent
	// checkbox in the workflow configure drawer — shown on first enable,
	// hidden thereafter.
	KeyWorkflowsConsentAckAt = "workflows.consent_acknowledged_at"

	// KeyNotifyWebhookURL is an optional outbound webhook URL. When set,
	// Breadbox POSTs a payload to it for workflow notifications (e.g.
	// a report a workflow flagged). Empty = notifications disabled. The
	// self-hoster controls the URL (point it at ntfy / Slack / Discord /
	// an email bridge). http(s) only.
	KeyNotifyWebhookURL = "notify.webhook_url"

	// KeyNotifyFormat selects how the outbound notification request is
	// shaped. "auto" (default) sniffs the webhook URL and publishes
	// natively to the matching provider (ntfy / Slack / Discord), falling
	// back to the generic JSON envelope otherwise. The explicit values
	// "ntfy" / "slack" / "discord" / "json" force a specific shape.
	KeyNotifyFormat = "notify.format"

	// KeyNotifyPublicBaseURL is the absolute origin (scheme+host, no
	// trailing slash) Breadbox prepends to report deep links carried in
	// a notification — so an ntfy "tap to open" (and any relative link
	// in the body) resolves to the real report instead of a bare path.
	// Empty = deep links stay relative. http(s) only.
	KeyNotifyPublicBaseURL = "notify.public_base_url"

	// KeyNotifyMinPriority gates outbound notifications by report priority:
	// only reports at or above this floor are delivered. One of
	// info | warning | critical; default "info" (everything). Lets a
	// household silence routine info-level reports and keep only alerts.
	KeyNotifyMinPriority = "notify.min_priority"

	// KeyNotifyChannels holds the JSON array of configured notification
	// channels (the multi-sink model). Each entry carries its own URL,
	// format, priority floor, optional ntfy token, enabled flag, and last
	// delivery status. When empty, a single legacy channel is synthesized
	// from KeyNotifyWebhookURL / KeyNotifyFormat / KeyNotifyMinPriority so
	// pre-multi-channel configs keep working. A workflow notification fans
	// out to every enabled channel.
	KeyNotifyChannels = "notify.channels"
)

// AuthMode values for KeyAgentAuthMode.
const (
	AuthModeSubscription = "subscription"
	AuthModeAPIKey       = "api_key"
)

// NotifyFormat values for KeyNotifyFormat.
const (
	NotifyFormatAuto       = "auto"
	NotifyFormatNtfy       = "ntfy"
	NotifyFormatSlack      = "slack"
	NotifyFormatDiscord    = "discord"
	NotifyFormatGoogleChat = "googlechat"
	NotifyFormatJSON       = "json"
)

// NotifyMinPriority values for KeyNotifyMinPriority (delivery floor).
const (
	NotifyMinPriorityInfo     = "info"
	NotifyMinPriorityWarning  = "warning"
	NotifyMinPriorityCritical = "critical"
)
