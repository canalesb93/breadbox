package config

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"breadbox/internal/crypto"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// Load reads configuration from the environment file and environment variables.
// It does not read from the database; call LoadWithDB after the pool is available.
func Load() (*Config, error) {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "local"
	}

	// Load the appropriate env file. Variables already set in the process
	// environment are not overridden.
	envFile := ".local.env"
	if env == "docker" {
		envFile = ".docker.env"
	}
	// Ignore error if the file doesn't exist; env vars may be set directly.
	_ = godotenv.Load(envFile)

	cfg := &Config{
		Environment:   env,
		ServerPort:    resolveServerPort(),
		LogLevel:      os.Getenv("LOG_LEVEL"),
		DataDir:       resolveDataDir(env),
		ConfigSources: make(map[string]string),
	}

	// Database URL: prefer DATABASE_URL, fall back to individual vars.
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		host := os.Getenv("DB_HOST")
		port := getEnv("DB_PORT", "5432")
		name := os.Getenv("DB_NAME")
		user := os.Getenv("DB_USER")
		password := os.Getenv("DB_PASSWORD")
		if host != "" && name != "" && user != "" {
			cfg.DatabaseURL = fmt.Sprintf(
				"postgres://%s:%s@%s:%s/%s?sslmode=disable",
				user, password, host, port, name,
			)
		}
	}

	// Encryption key: 64-char hex → 32 bytes.
	keyHex := os.Getenv("ENCRYPTION_KEY")
	if keyHex != "" {
		key, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, fmt.Errorf("ENCRYPTION_KEY: invalid hex: %w", err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("ENCRYPTION_KEY: expected 32 bytes, got %d", len(key))
		}
		cfg.EncryptionKey = key
	}

	// Plaid env-var overrides (may also come from app_config).
	cfg.PlaidClientID = os.Getenv("PLAID_CLIENT_ID")
	cfg.PlaidSecret = os.Getenv("PLAID_SECRET")
	cfg.PlaidEnv = os.Getenv("PLAID_ENV")
	if cfg.PlaidClientID != "" {
		cfg.ConfigSources["plaid_client_id"] = "env"
		cfg.ConfigSources["plaid_secret"] = "env"
		cfg.ConfigSources["plaid_env"] = "env"
	}

	// Teller env vars.
	cfg.TellerAppID = os.Getenv("TELLER_APP_ID")
	cfg.TellerCertPath = os.Getenv("TELLER_CERT_PATH")
	cfg.TellerKeyPath = os.Getenv("TELLER_KEY_PATH")
	cfg.TellerEnv = getEnv("TELLER_ENV", "sandbox")
	cfg.TellerWebhookSecret = os.Getenv("TELLER_WEBHOOK_SECRET")
	if cfg.TellerAppID != "" {
		cfg.ConfigSources["teller_app_id"] = "env"
		cfg.ConfigSources["teller_env"] = "env"
	}

	// SimpleFIN opt-in. If the env var is present at all it wins over app_config
	// (even when set to a falsey value), matching env-overrides-db precedence.
	if raw := os.Getenv("SIMPLEFIN_ENABLED"); raw != "" {
		cfg.SimpleFINEnabled = parseBool(raw)
		cfg.ConfigSources["simplefin_enabled"] = "env"
	}

	// Connection pool tuning.
	cfg.DBMaxConns = int32(getEnvInt("DB_MAX_CONNS", 25))
	cfg.DBMinConns = int32(getEnvInt("DB_MIN_CONNS", 2))
	cfg.DBMaxConnLifetimeM = getEnvInt("DB_MAX_CONN_LIFETIME_MINUTES", 60)

	// HTTP server timeouts.
	cfg.ReadTimeoutS = getEnvInt("HTTP_READ_TIMEOUT_SECONDS", 30)
	cfg.WriteTimeoutS = getEnvInt("HTTP_WRITE_TIMEOUT_SECONDS", 60)
	cfg.IdleTimeoutS = getEnvInt("HTTP_IDLE_TIMEOUT_SECONDS", 120)

	// Sync timeout.
	cfg.SyncTimeoutSeconds = getEnvInt("SYNC_TIMEOUT_SECONDS", 300)

	// API rate limit (per API key with IP fallback).
	cfg.APIRateLimitRPM = getEnvInt("API_RATE_LIMIT_RPM", 120)
	cfg.APIRateLimitBurst = getEnvInt("API_RATE_LIMIT_BURST", 60)

	// Dashboard gate. Truthy values: "1", "true", "yes", "on" (case-insensitive).
	cfg.NoDashboard = parseBool(os.Getenv("BREADBOX_NO_DASHBOARD"))

	// Session-cookie Secure policy: "always" | "never" | "auto" (default).
	cfg.SecureCookies = parseSecureCookieMode(os.Getenv("SECURE_COOKIES"))

	return cfg, nil
}

// parseSecureCookieMode maps SECURE_COOKIES to "always" | "never" | "auto".
// Unset or unrecognized → "auto" (the Secure flag follows the request scheme).
func parseSecureCookieMode(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on", "always", "secure":
		return "always"
	case "0", "false", "no", "off", "never", "insecure":
		return "never"
	default:
		return "auto"
	}
}

// parseBool returns true for "1", "true", "yes", "on" (case-insensitive); false otherwise.
func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// LoadWithDB merges app_config table values into the config. Environment
// variables take precedence: a value already set from the environment is not
// overwritten by the database.
func LoadWithDB(ctx context.Context, cfg *Config, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, "SELECT key, value FROM app_config")
	if err != nil {
		return fmt.Errorf("load app_config: %w", err)
	}
	defer rows.Close()

	appCfg := make(map[string]string)
	for rows.Next() {
		var k string
		var v *string
		if err := rows.Scan(&k, &v); err != nil {
			return fmt.Errorf("scan app_config row: %w", err)
		}
		if v != nil {
			appCfg[k] = *v
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate app_config: %w", err)
	}

	// Initialize ConfigSources if nil (in case Load() wasn't called first).
	if cfg.ConfigSources == nil {
		cfg.ConfigSources = make(map[string]string)
	}

	// Only set values that are not already provided by environment variables.
	if cfg.PlaidClientID == "" {
		cfg.PlaidClientID = appCfg["plaid_client_id"]
		if cfg.PlaidClientID != "" {
			cfg.ConfigSources["plaid_client_id"] = "db"
			cfg.ConfigSources["plaid_secret"] = "db"
			cfg.ConfigSources["plaid_env"] = "db"
		}
	}
	if cfg.PlaidSecret == "" {
		cfg.PlaidSecret = appCfg["plaid_secret"]
	}
	if cfg.PlaidEnv == "" {
		cfg.PlaidEnv = appCfg["plaid_env"]
		if cfg.PlaidEnv == "" {
			cfg.PlaidEnv = "sandbox"
		}
	}

	// Teller app_config fallbacks (cert/key paths and webhook secret are env-only).
	if cfg.TellerAppID == "" {
		cfg.TellerAppID = appCfg["teller_app_id"]
		if cfg.TellerAppID != "" {
			cfg.ConfigSources["teller_app_id"] = "db"
		}
	}
	if cfg.TellerEnv == "sandbox" && appCfg["teller_env"] != "" {
		cfg.TellerEnv = appCfg["teller_env"]
		if cfg.ConfigSources["teller_env"] == "" {
			cfg.ConfigSources["teller_env"] = "db"
		}
	}

	// Teller webhook secret from DB (if not from env).
	if cfg.TellerWebhookSecret == "" {
		cfg.TellerWebhookSecret = appCfg["teller_webhook_secret"]
	}

	// Teller certificate PEM from DB (encrypted, base64-encoded).
	// Only load if file paths are not set from env.
	if cfg.TellerCertPath == "" && cfg.TellerKeyPath == "" && len(cfg.EncryptionKey) > 0 {
		if v, ok := appCfg["teller_cert_pem"]; ok && v != "" {
			raw, err := base64.StdEncoding.DecodeString(v)
			if err == nil {
				dec, err := crypto.Decrypt(raw, cfg.EncryptionKey)
				if err == nil {
					cfg.TellerCertPEM = dec
				}
			}
		}
		if v, ok := appCfg["teller_key_pem"]; ok && v != "" {
			raw, err := base64.StdEncoding.DecodeString(v)
			if err == nil {
				dec, err := crypto.Decrypt(raw, cfg.EncryptionKey)
				if err == nil {
					cfg.TellerKeyPEM = dec
				}
			}
		}
	}

	// Sync interval from app_config.
	if v, ok := appCfg["sync_interval_minutes"]; ok {
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 {
			cfg.SyncIntervalMinutes = n
			cfg.ConfigSources["sync_interval_minutes"] = "db"
		}
	}
	if cfg.SyncIntervalMinutes == 0 {
		cfg.SyncIntervalMinutes = 12 * 60 // default 12h
		cfg.ConfigSources["sync_interval_minutes"] = "default"
	}

	cfg.WebhookURL = appCfg["webhook_url"]
	if cfg.WebhookURL != "" {
		cfg.ConfigSources["webhook_url"] = "db"
	} else {
		cfg.ConfigSources["webhook_url"] = "default"
	}

	// SimpleFIN opt-in from app_config (unless already set from env).
	if cfg.ConfigSources["simplefin_enabled"] != "env" {
		if v, ok := appCfg["simplefin_enabled"]; ok {
			cfg.SimpleFINEnabled = parseBool(v)
			cfg.ConfigSources["simplefin_enabled"] = "db"
		} else {
			cfg.ConfigSources["simplefin_enabled"] = "default"
		}
	}

	// Set defaults for any untracked config sources.
	for _, key := range []string{"plaid_client_id", "plaid_secret", "plaid_env", "teller_app_id", "teller_env"} {
		if cfg.ConfigSources[key] == "" {
			cfg.ConfigSources[key] = "default"
		}
	}

	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// resolveDataDir returns the persistent-data root used by transcripts,
// backups, and any other runtime-writable state. BB_DATA_DIR wins when
// set. Otherwise we default to /var/lib/breadbox in Docker (matches the
// FHS convention and what docker-compose.prod.yml + the Railway/Render
// one-click templates mount as a volume) and leave empty in local /
// CLI contexts so cwd-relative fallbacks apply.
func resolveDataDir(env string) string {
	if v := os.Getenv("BB_DATA_DIR"); v != "" {
		return v
	}
	if env == "docker" {
		return "/var/lib/breadbox"
	}
	return ""
}

// resolveServerPort returns the HTTP listen port. Tries SERVER_PORT first
// (the Breadbox-specific name), then PORT (the 12-factor convention used
// by Heroku, Fly.io, Railway, Render, Cloud Run, and most PaaS hosts),
// then defaults to 8080. The PORT fallback makes Breadbox deployable on
// any PaaS that injects a runtime port without the user having to
// translate it to SERVER_PORT.
func resolveServerPort() string {
	if v := os.Getenv("SERVER_PORT"); v != "" {
		return v
	}
	if v := os.Getenv("PORT"); v != "" {
		return v
	}
	return "8080"
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
