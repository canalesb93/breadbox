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

	// From app_config table only
	SyncIntervalHours int
	WebhookURL        string
	SetupComplete     bool
}
