package config

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"

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
		Environment: env,
		ServerPort:  getEnv("SERVER_PORT", "8080"),
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

	return cfg, nil
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

	// Only set values that are not already provided by environment variables.
	if cfg.PlaidClientID == "" {
		cfg.PlaidClientID = appCfg["plaid_client_id"]
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

	// Prefer sync_interval_minutes; fall back to sync_interval_hours (legacy).
	if v, ok := appCfg["sync_interval_minutes"]; ok {
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 {
			cfg.SyncIntervalMinutes = n
		}
	}
	if cfg.SyncIntervalMinutes == 0 {
		if v, ok := appCfg["sync_interval_hours"]; ok {
			n, err := strconv.Atoi(v)
			if err == nil && n > 0 {
				cfg.SyncIntervalMinutes = n * 60
			}
		}
	}
	if cfg.SyncIntervalMinutes == 0 {
		cfg.SyncIntervalMinutes = 12 * 60 // default 12h
	}

	cfg.WebhookURL = appCfg["webhook_url"]
	cfg.SetupComplete = appCfg["setup_complete"] == "true"

	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
