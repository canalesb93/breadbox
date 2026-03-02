package config

// Config holds all application configuration.
type Config struct {
	// Derived from environment
	DatabaseURL   string
	EncryptionKey []byte // 32 bytes, decoded from hex
	ServerPort    string
	Environment   string

	// May come from env (overrides app_config) or app_config table
	PlaidClientID string
	PlaidSecret   string
	PlaidEnv      string // "sandbox" | "development" | "production"

	// Teller — may come from env (overrides app_config) or app_config table
	TellerAppID        string
	TellerCertPath     string
	TellerKeyPath      string
	TellerEnv          string // "sandbox" | "production"
	TellerWebhookSecret string

	// From app_config table only
	SyncIntervalMinutes int
	WebhookURL          string
	SetupComplete       bool

	// Connection pool tuning (env vars only)
	DBMaxConns         int32 // DB_MAX_CONNS, default 25
	DBMinConns         int32 // DB_MIN_CONNS, default 2
	DBMaxConnLifetimeM int   // DB_MAX_CONN_LIFETIME_MINUTES, default 60

	// HTTP server timeouts (env vars only)
	ReadTimeoutS  int // HTTP_READ_TIMEOUT_SECONDS, default 30
	WriteTimeoutS int // HTTP_WRITE_TIMEOUT_SECONDS, default 60
	IdleTimeoutS  int // HTTP_IDLE_TIMEOUT_SECONDS, default 120
}
