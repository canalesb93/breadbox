package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear relevant env vars to test defaults.
	envVars := []string{
		"ENVIRONMENT", "SERVER_PORT", "DATABASE_URL", "ENCRYPTION_KEY",
		"PLAID_CLIENT_ID", "PLAID_SECRET", "PLAID_ENV",
		"TELLER_APP_ID", "TELLER_CERT_PATH", "TELLER_KEY_PATH", "TELLER_ENV", "TELLER_WEBHOOK_SECRET",
		"DB_MAX_CONNS", "DB_MIN_CONNS", "DB_MAX_CONN_LIFETIME_MINUTES",
		"HTTP_READ_TIMEOUT_SECONDS", "HTTP_WRITE_TIMEOUT_SECONDS", "HTTP_IDLE_TIMEOUT_SECONDS",
		"SYNC_TIMEOUT_SECONDS", "LOG_LEVEL",
		"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD",
	}
	saved := make(map[string]string)
	for _, key := range envVars {
		saved[key] = os.Getenv(key)
		os.Unsetenv(key)
	}
	t.Cleanup(func() {
		for key, val := range saved {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Environment != "local" {
		t.Errorf("expected environment 'local', got %q", cfg.Environment)
	}
	if cfg.ServerPort != "8080" {
		t.Errorf("expected server port '8080', got %q", cfg.ServerPort)
	}
	if cfg.DBMaxConns != 25 {
		t.Errorf("expected DBMaxConns 25, got %d", cfg.DBMaxConns)
	}
	if cfg.DBMinConns != 2 {
		t.Errorf("expected DBMinConns 2, got %d", cfg.DBMinConns)
	}
	if cfg.DBMaxConnLifetimeM != 60 {
		t.Errorf("expected DBMaxConnLifetimeM 60, got %d", cfg.DBMaxConnLifetimeM)
	}
	if cfg.ReadTimeoutS != 30 {
		t.Errorf("expected ReadTimeoutS 30, got %d", cfg.ReadTimeoutS)
	}
	if cfg.WriteTimeoutS != 60 {
		t.Errorf("expected WriteTimeoutS 60, got %d", cfg.WriteTimeoutS)
	}
	if cfg.IdleTimeoutS != 120 {
		t.Errorf("expected IdleTimeoutS 120, got %d", cfg.IdleTimeoutS)
	}
	if cfg.SyncTimeoutSeconds != 300 {
		t.Errorf("expected SyncTimeoutSeconds 300, got %d", cfg.SyncTimeoutSeconds)
	}
	if cfg.ConfigSources == nil {
		t.Error("expected ConfigSources to be initialized")
	}
}

func TestLoad_EncryptionKey_Valid(t *testing.T) {
	saved := os.Getenv("ENCRYPTION_KEY")
	t.Cleanup(func() {
		if saved != "" {
			os.Setenv("ENCRYPTION_KEY", saved)
		} else {
			os.Unsetenv("ENCRYPTION_KEY")
		}
	})

	// 64 hex chars = 32 bytes
	os.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.EncryptionKey) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(cfg.EncryptionKey))
	}
}

func TestLoad_EncryptionKey_InvalidHex(t *testing.T) {
	saved := os.Getenv("ENCRYPTION_KEY")
	t.Cleanup(func() {
		if saved != "" {
			os.Setenv("ENCRYPTION_KEY", saved)
		} else {
			os.Unsetenv("ENCRYPTION_KEY")
		}
	})

	os.Setenv("ENCRYPTION_KEY", "not-valid-hex-string")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid hex")
	}
}

func TestLoad_EncryptionKey_WrongLength(t *testing.T) {
	saved := os.Getenv("ENCRYPTION_KEY")
	t.Cleanup(func() {
		if saved != "" {
			os.Setenv("ENCRYPTION_KEY", saved)
		} else {
			os.Unsetenv("ENCRYPTION_KEY")
		}
	})

	os.Setenv("ENCRYPTION_KEY", "0123456789abcdef") // only 16 hex chars = 8 bytes
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for wrong key length")
	}
}

func TestLoad_ServerPort_FromEnv(t *testing.T) {
	saved := os.Getenv("SERVER_PORT")
	t.Cleanup(func() {
		if saved != "" {
			os.Setenv("SERVER_PORT", saved)
		} else {
			os.Unsetenv("SERVER_PORT")
		}
	})

	os.Setenv("SERVER_PORT", "9090")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.ServerPort != "9090" {
		t.Errorf("expected port '9090', got %q", cfg.ServerPort)
	}
}

func TestLoad_PlaidEnvVars(t *testing.T) {
	vars := map[string]string{
		"PLAID_CLIENT_ID": "test-client",
		"PLAID_SECRET":    "test-secret",
		"PLAID_ENV":       "sandbox",
	}
	saved := make(map[string]string)
	for key := range vars {
		saved[key] = os.Getenv(key)
	}
	t.Cleanup(func() {
		for key, val := range saved {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	})

	for key, val := range vars {
		os.Setenv(key, val)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.PlaidClientID != "test-client" {
		t.Errorf("expected PlaidClientID 'test-client', got %q", cfg.PlaidClientID)
	}
	if cfg.PlaidSecret != "test-secret" {
		t.Errorf("expected PlaidSecret 'test-secret', got %q", cfg.PlaidSecret)
	}
	if cfg.PlaidEnv != "sandbox" {
		t.Errorf("expected PlaidEnv 'sandbox', got %q", cfg.PlaidEnv)
	}
	if cfg.ConfigSources["plaid_client_id"] != "env" {
		t.Errorf("expected plaid_client_id source 'env', got %q", cfg.ConfigSources["plaid_client_id"])
	}
}

func TestLoad_DatabaseURL_FromIndividualVars(t *testing.T) {
	vars := []string{"DATABASE_URL", "DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD"}
	saved := make(map[string]string)
	for _, key := range vars {
		saved[key] = os.Getenv(key)
		os.Unsetenv(key)
	}
	t.Cleanup(func() {
		for key, val := range saved {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	})

	os.Setenv("DB_HOST", "myhost")
	os.Setenv("DB_PORT", "5433")
	os.Setenv("DB_NAME", "mydb")
	os.Setenv("DB_USER", "myuser")
	os.Setenv("DB_PASSWORD", "mypass")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	want := "postgres://myuser:mypass@myhost:5433/mydb?sslmode=disable"
	if cfg.DatabaseURL != want {
		t.Errorf("expected DatabaseURL %q, got %q", want, cfg.DatabaseURL)
	}
}

func TestLoad_TellerEnvVars(t *testing.T) {
	vars := map[string]string{
		"TELLER_APP_ID":         "teller-app-123",
		"TELLER_ENV":            "production",
		"TELLER_WEBHOOK_SECRET": "webhook-secret",
	}
	saved := make(map[string]string)
	for key := range vars {
		saved[key] = os.Getenv(key)
	}
	t.Cleanup(func() {
		for key, val := range saved {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	})

	for key, val := range vars {
		os.Setenv(key, val)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.TellerAppID != "teller-app-123" {
		t.Errorf("expected TellerAppID 'teller-app-123', got %q", cfg.TellerAppID)
	}
	if cfg.TellerEnv != "production" {
		t.Errorf("expected TellerEnv 'production', got %q", cfg.TellerEnv)
	}
	if cfg.TellerWebhookSecret != "webhook-secret" {
		t.Errorf("expected TellerWebhookSecret 'webhook-secret', got %q", cfg.TellerWebhookSecret)
	}
	if cfg.ConfigSources["teller_app_id"] != "env" {
		t.Errorf("expected teller_app_id source 'env', got %q", cfg.ConfigSources["teller_app_id"])
	}
}

func TestLoad_IntEnvVars(t *testing.T) {
	vars := map[string]string{
		"DB_MAX_CONNS":              "50",
		"DB_MIN_CONNS":              "5",
		"DB_MAX_CONN_LIFETIME_MINUTES": "120",
		"HTTP_READ_TIMEOUT_SECONDS":    "10",
		"HTTP_WRITE_TIMEOUT_SECONDS":   "20",
		"HTTP_IDLE_TIMEOUT_SECONDS":    "30",
		"SYNC_TIMEOUT_SECONDS":         "600",
	}
	saved := make(map[string]string)
	for key := range vars {
		saved[key] = os.Getenv(key)
	}
	t.Cleanup(func() {
		for key, val := range saved {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	})

	for key, val := range vars {
		os.Setenv(key, val)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.DBMaxConns != 50 {
		t.Errorf("expected DBMaxConns 50, got %d", cfg.DBMaxConns)
	}
	if cfg.DBMinConns != 5 {
		t.Errorf("expected DBMinConns 5, got %d", cfg.DBMinConns)
	}
	if cfg.DBMaxConnLifetimeM != 120 {
		t.Errorf("expected DBMaxConnLifetimeM 120, got %d", cfg.DBMaxConnLifetimeM)
	}
	if cfg.ReadTimeoutS != 10 {
		t.Errorf("expected ReadTimeoutS 10, got %d", cfg.ReadTimeoutS)
	}
	if cfg.WriteTimeoutS != 20 {
		t.Errorf("expected WriteTimeoutS 20, got %d", cfg.WriteTimeoutS)
	}
	if cfg.IdleTimeoutS != 30 {
		t.Errorf("expected IdleTimeoutS 30, got %d", cfg.IdleTimeoutS)
	}
	if cfg.SyncTimeoutSeconds != 600 {
		t.Errorf("expected SyncTimeoutSeconds 600, got %d", cfg.SyncTimeoutSeconds)
	}
}

func TestLoad_IntEnvVars_InvalidFallsBack(t *testing.T) {
	saved := os.Getenv("DB_MAX_CONNS")
	t.Cleanup(func() {
		if saved != "" {
			os.Setenv("DB_MAX_CONNS", saved)
		} else {
			os.Unsetenv("DB_MAX_CONNS")
		}
	})

	os.Setenv("DB_MAX_CONNS", "not-a-number")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	// Should fall back to default
	if cfg.DBMaxConns != 25 {
		t.Errorf("expected DBMaxConns 25 (default), got %d", cfg.DBMaxConns)
	}
}

func TestGetEnv_WithValue(t *testing.T) {
	os.Setenv("TEST_GETENV_BREADBOX", "found")
	t.Cleanup(func() { os.Unsetenv("TEST_GETENV_BREADBOX") })

	got := getEnv("TEST_GETENV_BREADBOX", "fallback")
	if got != "found" {
		t.Errorf("expected 'found', got %q", got)
	}
}

func TestGetEnv_Fallback(t *testing.T) {
	os.Unsetenv("TEST_GETENV_MISSING_BREADBOX")
	got := getEnv("TEST_GETENV_MISSING_BREADBOX", "fallback")
	if got != "fallback" {
		t.Errorf("expected 'fallback', got %q", got)
	}
}

func TestGetEnvInt_WithValue(t *testing.T) {
	os.Setenv("TEST_GETENVINT_BREADBOX", "42")
	t.Cleanup(func() { os.Unsetenv("TEST_GETENVINT_BREADBOX") })

	got := getEnvInt("TEST_GETENVINT_BREADBOX", 10)
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

func TestGetEnvInt_Fallback(t *testing.T) {
	os.Unsetenv("TEST_GETENVINT_MISSING_BREADBOX")
	got := getEnvInt("TEST_GETENVINT_MISSING_BREADBOX", 10)
	if got != 10 {
		t.Errorf("expected 10, got %d", got)
	}
}

func TestGetEnvInt_InvalidFallsBack(t *testing.T) {
	os.Setenv("TEST_GETENVINT_BAD_BREADBOX", "abc")
	t.Cleanup(func() { os.Unsetenv("TEST_GETENVINT_BAD_BREADBOX") })

	got := getEnvInt("TEST_GETENVINT_BAD_BREADBOX", 10)
	if got != 10 {
		t.Errorf("expected 10 (fallback), got %d", got)
	}
}
