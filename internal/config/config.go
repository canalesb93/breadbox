package config

import "time"

// Config holds all application configuration.
type Config struct {
	// Derived from environment
	DatabaseURL   string
	EncryptionKey []byte // 32 bytes, decoded from hex
	ServerPort    string
	Environment   string
	LogLevel      string // LOG_LEVEL: debug, info, warn, error

	// SecureCookies controls the Secure attribute on the session cookie:
	// "always", "never", or "auto" (default). In "auto" the Secure flag
	// follows the actual request scheme (direct HTTPS, or a trusted proxy's
	// X-Forwarded-Proto=https), so a plain-HTTP LAN/localhost install can
	// still log in while HTTPS deployments stay hardened. From SECURE_COOKIES;
	// applied per request by middleware.SecureSessionCookie.
	SecureCookies string

	// DataDir is the root directory for persistent runtime data (agent
	// transcripts, scheduled pg_dump backups, future cached blobs).
	// Sourced from BB_DATA_DIR; defaults to "/var/lib/breadbox" when
	// ENVIRONMENT=docker, empty otherwise. When empty, per-subsystem
	// defaults fall back to cwd-relative paths. Per-subsystem env vars
	// (BREADBOX_AGENT_TRANSCRIPT_DIR, BACKUP_DIR) still override.
	DataDir string

	// May come from env (overrides app_config) or app_config table
	PlaidClientID string
	PlaidSecret   string
	PlaidEnv      string // "sandbox" | "development" | "production"

	// Teller — may come from env (overrides app_config) or app_config table
	TellerAppID         string
	TellerCertPath      string // file path from env
	TellerKeyPath       string // file path from env
	TellerCertPEM       []byte // PEM bytes from DB (encrypted)
	TellerKeyPEM        []byte // PEM bytes from DB (encrypted)
	TellerEnv           string // "sandbox" | "production"
	TellerWebhookSecret string

	// SimpleFIN — opt-in toggle (env SIMPLEFIN_ENABLED overrides app_config).
	// SimpleFIN has no server-level credential: the access token is per
	// connection (pasted at connect time), so the only global config is whether
	// the provider is offered as a connect option at all.
	SimpleFINEnabled bool

	// From app_config table only
	SyncIntervalMinutes int
	WebhookURL          string

	// Connection pool tuning (env vars only)
	DBMaxConns         int32 // DB_MAX_CONNS, default 25
	DBMinConns         int32 // DB_MIN_CONNS, default 2
	DBMaxConnLifetimeM int   // DB_MAX_CONN_LIFETIME_MINUTES, default 60

	// HTTP server timeouts (env vars only)
	ReadTimeoutS  int // HTTP_READ_TIMEOUT_SECONDS, default 30
	WriteTimeoutS int // HTTP_WRITE_TIMEOUT_SECONDS, default 60
	IdleTimeoutS  int // HTTP_IDLE_TIMEOUT_SECONDS, default 120

	// Sync timeout (env vars only)
	SyncTimeoutSeconds int // SYNC_TIMEOUT_SECONDS, default 300

	// API rate limit (env vars only). Applies to /api/v1/* per API key (with
	// IP fallback). Defaults match middleware.DefaultRateLimitRPM /
	// middleware.DefaultRateLimitBurst (120 req/min, burst 60).
	APIRateLimitRPM   int // API_RATE_LIMIT_RPM, default 120
	APIRateLimitBurst int // API_RATE_LIMIT_BURST, default 60

	// Runtime gates. NoDashboard, when true, prevents the HTTP router from
	// registering the admin dashboard. REST, MCP, OAuth discovery, and
	// webhooks stay up. Set by `breadbox serve --no-dashboard` or
	// BREADBOX_NO_DASHBOARD=true. The build-tag side that strips the assets
	// entirely is `-tags=headless` (see .claude/rules/build-tags.md).
	NoDashboard bool

	// Runtime metadata (set at startup)
	Version   string
	StartTime time.Time

	// Config source tracking: key → "env" | "db" | "default"
	ConfigSources map[string]string
}
