package config

import "time"

// Config holds all application configuration.
type Config struct {
	// Derived from environment
	DatabaseURL   string
	EncryptionKey []byte // 32 bytes, decoded from hex (env) or random bytes (file/generated)
	// EncryptionKeySource describes where the live key came from:
	//   "env"        — ENCRYPTION_KEY env var (BYO)
	//   "file"       — read from ${BREADBOX_DATA_DIR}/encryption.key
	//   "generated"  — freshly generated this boot and written to the data dir
	//   ""           — no key (no provider configured, env unset, generation skipped)
	EncryptionKeySource string
	// EncryptionKeyPath is the on-disk location consulted/used for the file/generated
	// sources. Empty when the key came from env or no key is set.
	EncryptionKeyPath string
	// DataDir is the resolved data directory used for the encryption key file
	// (and any future per-install state). Sourced from BREADBOX_DATA_DIR with a
	// docker (/data) vs local (./data) default.
	DataDir     string
	ServerPort  string
	Environment string
	LogLevel    string // LOG_LEVEL: debug, info, warn, error

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

	// Runtime metadata (set at startup)
	Version   string
	StartTime time.Time

	// Config source tracking: key → "env" | "db" | "default"
	ConfigSources map[string]string
}
