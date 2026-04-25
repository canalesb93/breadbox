package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// withCleanDataDir points BREADBOX_DATA_DIR at a t.TempDir() so the auto-key
// resolution path doesn't pollute the working tree, and clears ENCRYPTION_KEY
// unless the test sets it.
func withCleanDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BREADBOX_DATA_DIR", dir)
	t.Setenv("ENCRYPTION_KEY", "")
	return dir
}

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
		"BREADBOX_DATA_DIR",
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

	// Point at a tmp data dir so the auto-key resolver doesn't write to ./data.
	t.Setenv("BREADBOX_DATA_DIR", t.TempDir())

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
	withCleanDataDir(t)
	// 64 hex chars = 32 bytes
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.EncryptionKey) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(cfg.EncryptionKey))
	}
	if cfg.EncryptionKeySource != EncryptionKeySourceEnv {
		t.Errorf("expected source=env, got %q", cfg.EncryptionKeySource)
	}
	if cfg.EncryptionKeyPath != "" {
		t.Errorf("expected empty path for env source, got %q", cfg.EncryptionKeyPath)
	}
}

func TestLoad_EncryptionKey_InvalidHex(t *testing.T) {
	withCleanDataDir(t)
	t.Setenv("ENCRYPTION_KEY", "not-valid-hex-string")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid hex")
	}
}

func TestLoad_EncryptionKey_WrongLength(t *testing.T) {
	withCleanDataDir(t)
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef") // only 16 hex chars = 8 bytes
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for wrong key length")
	}
}

// TestLoad_EncryptionKey_GeneratesWhenMissing covers the auto-managed key path:
// no env var, no existing file, ${BREADBOX_DATA_DIR}/encryption.key gets created
// atomically with mode 0600 and Source=="generated".
func TestLoad_EncryptionKey_GeneratesWhenMissing(t *testing.T) {
	dir := withCleanDataDir(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.EncryptionKey) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(cfg.EncryptionKey))
	}
	if cfg.EncryptionKeySource != EncryptionKeySourceGenerated {
		t.Fatalf("expected source=generated, got %q", cfg.EncryptionKeySource)
	}

	wantPath := filepath.Join(dir, EncryptionKeyFilename)
	if cfg.EncryptionKeyPath != wantPath {
		t.Fatalf("path: got %q, want %q", cfg.EncryptionKeyPath, wantPath)
	}

	info, err := os.Stat(wantPath)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	// File mode should be 0600. On macOS/Linux this is enforced by atomicWriteKeyFile.
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("key file perms: got %v, want 0600", mode)
	}
}

// TestLoad_EncryptionKey_ReadsExistingFile covers the second path in the
// resolution order: a pre-existing key file is preferred over auto-generation
// and reported with source="file".
func TestLoad_EncryptionKey_ReadsExistingFile(t *testing.T) {
	dir := withCleanDataDir(t)

	wantKey := make([]byte, 32)
	if _, err := rand.Read(wantKey); err != nil {
		t.Fatalf("rand: %v", err)
	}
	keyPath := filepath.Join(dir, EncryptionKeyFilename)
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(wantKey)+"\n"), 0o600); err != nil {
		t.Fatalf("seed key file: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.EncryptionKeySource != EncryptionKeySourceFile {
		t.Fatalf("expected source=file, got %q", cfg.EncryptionKeySource)
	}
	if cfg.EncryptionKeyPath != keyPath {
		t.Fatalf("path: got %q, want %q", cfg.EncryptionKeyPath, keyPath)
	}
	if string(cfg.EncryptionKey) != string(wantKey) {
		t.Fatalf("key bytes mismatch: %x vs %x", cfg.EncryptionKey, wantKey)
	}
}

// TestLoad_EncryptionKey_EnvWinsOverFile makes sure operators who set
// ENCRYPTION_KEY in the environment keep working unchanged — the file path is
// ignored entirely (and never even read) in that case.
func TestLoad_EncryptionKey_EnvWinsOverFile(t *testing.T) {
	dir := withCleanDataDir(t)

	// Write a different key into the file to prove we don't pick it up.
	fileKey := make([]byte, 32)
	for i := range fileKey {
		fileKey[i] = 0xAA
	}
	keyPath := filepath.Join(dir, EncryptionKeyFilename)
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(fileKey)+"\n"), 0o600); err != nil {
		t.Fatalf("seed key file: %v", err)
	}

	envHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	t.Setenv("ENCRYPTION_KEY", envHex)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.EncryptionKeySource != EncryptionKeySourceEnv {
		t.Fatalf("expected source=env, got %q", cfg.EncryptionKeySource)
	}
	wantBytes, _ := hex.DecodeString(envHex)
	if string(cfg.EncryptionKey) != string(wantBytes) {
		t.Fatalf("loaded env key does not match: got %x", cfg.EncryptionKey)
	}
}

// TestLoad_DataDir_Defaults verifies the per-environment fallback when
// BREADBOX_DATA_DIR is unset.
func TestLoad_DataDir_Defaults(t *testing.T) {
	tests := []struct {
		env     string
		wantDir string
	}{
		{"docker", "/data"},
		{"local", filepath.Join(".", "data")},
	}
	for _, tc := range tests {
		t.Run(tc.env, func(t *testing.T) {
			if got := defaultDataDir(tc.env); got != tc.wantDir {
				t.Fatalf("defaultDataDir(%q) = %q, want %q", tc.env, got, tc.wantDir)
			}
		})
	}
}

// TestLoad_DataDir_FromEnv verifies that BREADBOX_DATA_DIR overrides the per-
// environment fallback.
func TestLoad_DataDir_FromEnv(t *testing.T) {
	want := withCleanDataDir(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.DataDir != want {
		t.Fatalf("DataDir: got %q, want %q", cfg.DataDir, want)
	}
}

func TestLoad_ServerPort_FromEnv(t *testing.T) {
	withCleanDataDir(t)
	t.Setenv("SERVER_PORT", "9090")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.ServerPort != "9090" {
		t.Errorf("expected port '9090', got %q", cfg.ServerPort)
	}
}

func TestLoad_PlaidEnvVars(t *testing.T) {
	withCleanDataDir(t)
	vars := map[string]string{
		"PLAID_CLIENT_ID": "test-client",
		"PLAID_SECRET":    "test-secret",
		"PLAID_ENV":       "sandbox",
	}
	for key, val := range vars {
		t.Setenv(key, val)
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
	withCleanDataDir(t)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_HOST", "myhost")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_NAME", "mydb")
	t.Setenv("DB_USER", "myuser")
	t.Setenv("DB_PASSWORD", "mypass")

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
	withCleanDataDir(t)
	vars := map[string]string{
		"TELLER_APP_ID":         "teller-app-123",
		"TELLER_ENV":            "production",
		"TELLER_WEBHOOK_SECRET": "webhook-secret",
	}
	for key, val := range vars {
		t.Setenv(key, val)
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
	withCleanDataDir(t)
	vars := map[string]string{
		"DB_MAX_CONNS":                 "50",
		"DB_MIN_CONNS":                 "5",
		"DB_MAX_CONN_LIFETIME_MINUTES": "120",
		"HTTP_READ_TIMEOUT_SECONDS":    "10",
		"HTTP_WRITE_TIMEOUT_SECONDS":   "20",
		"HTTP_IDLE_TIMEOUT_SECONDS":    "30",
		"SYNC_TIMEOUT_SECONDS":         "600",
	}
	for key, val := range vars {
		t.Setenv(key, val)
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
	withCleanDataDir(t)
	t.Setenv("DB_MAX_CONNS", "not-a-number")
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
