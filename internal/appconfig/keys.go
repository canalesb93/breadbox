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
)

// AuthMode values for KeyAgentAuthMode.
const (
	AuthModeSubscription = "subscription"
	AuthModeAPIKey       = "api_key"
)
