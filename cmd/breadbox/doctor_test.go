package main

import (
	"os"
	"strings"
	"testing"

	"breadbox/internal/config"
)

func TestCheckEncryptionKey(t *testing.T) {
	tests := []struct {
		name       string
		envKey     string
		cfg        *config.Config
		wantStatus string
		wantMsgSub string
	}{
		{
			name:       "unset and no provider skips",
			envKey:     "",
			cfg:        &config.Config{},
			wantStatus: doctorStatusSkip,
		},
		{
			name:       "unset with plaid fails",
			envKey:     "",
			cfg:        &config.Config{PlaidClientID: "id"},
			wantStatus: doctorStatusFail,
			wantMsgSub: "ENCRYPTION_KEY is required",
		},
		{
			name:       "invalid hex fails",
			envKey:     "zzzz",
			cfg:        &config.Config{},
			wantStatus: doctorStatusFail,
			wantMsgSub: "not valid hex",
		},
		{
			name:       "wrong length fails",
			envKey:     "deadbeef",
			cfg:        &config.Config{},
			wantStatus: doctorStatusFail,
			wantMsgSub: "must decode to 32 bytes",
		},
		{
			name:       "valid 32-byte hex passes",
			envKey:     strings.Repeat("ab", 32),
			cfg:        &config.Config{},
			wantStatus: doctorStatusPass,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENCRYPTION_KEY", tc.envKey)
			got := checkEncryptionKey(tc.cfg)
			if got.Status != tc.wantStatus {
				t.Fatalf("status: got %q, want %q (msg=%q)", got.Status, tc.wantStatus, got.Message)
			}
			if tc.wantMsgSub != "" && !strings.Contains(got.Message, tc.wantMsgSub) {
				t.Fatalf("message: got %q, want substring %q", got.Message, tc.wantMsgSub)
			}
		})
	}
}

func TestCheckPlaid(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *config.Config
		wantStatus string
	}{
		{"missing secret", &config.Config{PlaidClientID: "id", PlaidEnv: "sandbox"}, doctorStatusFail},
		{"empty env", &config.Config{PlaidClientID: "id", PlaidSecret: "s"}, doctorStatusFail},
		{"bogus env", &config.Config{PlaidClientID: "id", PlaidSecret: "s", PlaidEnv: "staging"}, doctorStatusFail},
		{"sandbox ok", &config.Config{PlaidClientID: "id", PlaidSecret: "s", PlaidEnv: "sandbox"}, doctorStatusPass},
		{"production ok", &config.Config{PlaidClientID: "id", PlaidSecret: "s", PlaidEnv: "production"}, doctorStatusPass},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkPlaid(tc.cfg).Status; got != tc.wantStatus {
				t.Fatalf("status: got %q, want %q", got, tc.wantStatus)
			}
		})
	}
}

func TestCheckTeller(t *testing.T) {
	// Create a readable temp file to stand in as a cert path.
	tmpCert := t.TempDir() + "/cert.pem"
	if err := writeFile(tmpCert, []byte("-----BEGIN CERT-----\n")); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		cfg        *config.Config
		wantStatus string
	}{
		{
			name:       "bad env",
			cfg:        &config.Config{TellerAppID: "id", TellerEnv: "weird"},
			wantStatus: doctorStatusFail,
		},
		{
			name:       "no cert material",
			cfg:        &config.Config{TellerAppID: "id", TellerEnv: "sandbox"},
			wantStatus: doctorStatusFail,
		},
		{
			name:       "missing file fails",
			cfg:        &config.Config{TellerAppID: "id", TellerEnv: "sandbox", TellerCertPath: "/nope/nonexistent.pem"},
			wantStatus: doctorStatusFail,
		},
		{
			name:       "readable cert path passes",
			cfg:        &config.Config{TellerAppID: "id", TellerEnv: "sandbox", TellerCertPath: tmpCert, TellerKeyPath: tmpCert},
			wantStatus: doctorStatusPass,
		},
		{
			name:       "DB-stored PEM passes",
			cfg:        &config.Config{TellerAppID: "id", TellerEnv: "sandbox", TellerCertPEM: []byte("x"), TellerKeyPEM: []byte("y")},
			wantStatus: doctorStatusPass,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkTeller(tc.cfg).Status; got != tc.wantStatus {
				t.Fatalf("status: got %q, want %q", got, tc.wantStatus)
			}
		})
	}
}

func TestCheckCronConfig(t *testing.T) {
	t.Run("valid default sync interval", func(t *testing.T) {
		got := checkCronConfig(&config.Config{SyncIntervalMinutes: 60})
		if got.Status != doctorStatusPass {
			t.Fatalf("status: got %q, want pass (msg=%q)", got.Status, got.Message)
		}
	})
	t.Run("invalid BACKUP_CRON fails", func(t *testing.T) {
		t.Setenv("BACKUP_CRON", "not-a-cron")
		got := checkCronConfig(&config.Config{SyncIntervalMinutes: 60})
		if got.Status != doctorStatusFail {
			t.Fatalf("status: got %q, want fail", got.Status)
		}
	})
	t.Run("valid BACKUP_CRON passes", func(t *testing.T) {
		t.Setenv("BACKUP_CRON", "0 2 * * *")
		got := checkCronConfig(&config.Config{SyncIntervalMinutes: 60})
		if got.Status != doctorStatusPass {
			t.Fatalf("status: got %q, want pass (msg=%q)", got.Status, got.Message)
		}
	})
}

func TestCheckPublicURL(t *testing.T) {
	t.Run("unset skips", func(t *testing.T) {
		t.Setenv("PUBLIC_URL", "")
		t.Setenv("DOMAIN", "")
		if got := checkPublicURL(false).Status; got != doctorStatusSkip {
			t.Fatalf("status: got %q, want skip", got)
		}
	})
	t.Run("skip-external flag short-circuits", func(t *testing.T) {
		t.Setenv("PUBLIC_URL", "https://example.com")
		if got := checkPublicURL(true).Status; got != doctorStatusSkip {
			t.Fatalf("status: got %q, want skip", got)
		}
	})
}

func TestLatestEmbeddedMigration(t *testing.T) {
	v, err := latestEmbeddedMigration()
	if err != nil {
		t.Fatalf("latestEmbeddedMigration: %v", err)
	}
	if v <= 0 {
		t.Fatalf("expected a positive version, got %d", v)
	}
}

// helpers

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0600)
}
