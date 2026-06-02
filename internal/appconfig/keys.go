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
	// one. Default: "glyphs" (DiceBear's v10 glyph identicons).
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
	// natively to ntfy when it looks like ntfy, falling back to the
	// generic JSON envelope otherwise; "ntfy" forces ntfy's
	// header+body publishing; "json" forces the JSON envelope (for
	// Slack-compatible relays, Discord bridges, custom consumers).
	KeyNotifyFormat = "notify.format"

	// KeyNotifyPublicBaseURL is the absolute origin (scheme+host, no
	// trailing slash) Breadbox prepends to report deep links carried in
	// a notification — so an ntfy "tap to open" (and any relative link
	// in the body) resolves to the real report instead of a bare path.
	// Empty = deep links stay relative. http(s) only.
	KeyNotifyPublicBaseURL = "notify.public_base_url"

	// KeyDevModeEnabled gates the in-app Developer Mode reporter — a
	// floating bug/task filer rendered on every page that captures a
	// screenshot + HTML snapshot of the current screen and opens a
	// labelled GitHub issue. "true" enables it; anything else (default)
	// keeps it off. Internal/self-host tooling; off by default.
	KeyDevModeEnabled = "devmode.enabled"

	// KeyDevModeGithubRepo is the "owner/repo" the reporter files issues
	// against (e.g. "canalesb93/breadbox"). Empty disables filing.
	KeyDevModeGithubRepo = "devmode.github_repo"

	// KeyDevModeGithubToken stores the encrypted GitHub token used to
	// create issues. Needs the classic `repo` scope, or a fine-grained
	// token with read+write "Issues" permission on the target repo.
	KeyDevModeGithubToken = "devmode.github_token"

	// KeyDevModeIssueLabel is the label applied to every filed issue. The
	// reporter creates it on the repo if it doesn't exist yet. Default:
	// DevModeDefaultLabel.
	KeyDevModeIssueLabel = "devmode.issue_label"
)

// DevModeDefaultLabel is the label applied to filed Developer Mode issues
// when KeyDevModeIssueLabel is unset.
const DevModeDefaultLabel = "dev-report"

// AuthMode values for KeyAgentAuthMode.
const (
	AuthModeSubscription = "subscription"
	AuthModeAPIKey       = "api_key"
)

// NotifyFormat values for KeyNotifyFormat.
const (
	NotifyFormatAuto = "auto"
	NotifyFormatNtfy = "ntfy"
	NotifyFormatJSON = "json"
)
