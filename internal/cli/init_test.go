package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGatherInitInputs_NonInteractiveRequiresEmailPassword(t *testing.T) {
	_, _, _, err := gatherInitInputs(initOpts{NonInteractive: true})
	if err == nil {
		t.Fatal("expected error when --non-interactive but no email/password")
	}
	if !strings.Contains(err.Error(), "--email") {
		t.Errorf("unexpected error %q; want hint about --email", err.Error())
	}
}

func TestGatherInitInputs_NonInteractiveDerivesUserName(t *testing.T) {
	email, _, name, err := gatherInitInputs(initOpts{
		NonInteractive: true,
		Email:          "Alice@example.com",
		Password:       "super-secret-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "Alice@example.com" {
		t.Errorf("email = %q, want Alice@example.com", email)
	}
	if name != "Alice" {
		t.Errorf("name = %q, want Alice (derived from email local-part)", name)
	}
}

func TestGatherInitInputs_RejectsShortPassword(t *testing.T) {
	_, _, _, err := gatherInitInputs(initOpts{
		NonInteractive: true,
		Email:          "alice@example.com",
		Password:       "short",
	})
	if err == nil || !strings.Contains(err.Error(), "8 characters") {
		t.Fatalf("expected min-length error, got %v", err)
	}
}

func TestGatherInitInputs_RejectsBadEmail(t *testing.T) {
	_, _, _, err := gatherInitInputs(initOpts{
		NonInteractive: true,
		Email:          "not-an-email",
		Password:       "longenough123",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid email") {
		t.Fatalf("expected invalid-email error, got %v", err)
	}
}

func TestHasExistingKey(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"PORT=8080\nENCRYPTION_KEY=\n", false},
		{"# ENCRYPTION_KEY=fake\nPORT=8080\n", false},
		{"ENCRYPTION_KEY=abc123\n", true},
		{"ENCRYPTION_KEY=\"abc\"\n", true},
		{"PORT=8080\nENCRYPTION_KEY=abc\n", true},
	}
	for _, c := range cases {
		got := hasExistingKey(c.in)
		if got != c.want {
			t.Errorf("hasExistingKey(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestWriteEncryptionKey_AppendsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := writeEncryptionKey(path, "deadbeef"); err != nil {
		t.Fatalf("writeEncryptionKey: %v", err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "ENCRYPTION_KEY=deadbeef") {
		t.Fatalf("env file missing key: %q", string(body))
	}
}

func TestWriteEncryptionKey_PreservesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	const existing = "PORT=8080\nENCRYPTION_KEY=existingkey\n"
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeEncryptionKey(path, "newkey"); err != nil {
		t.Fatalf("writeEncryptionKey: %v", err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "ENCRYPTION_KEY=existingkey") {
		t.Errorf("existing key dropped: %q", string(body))
	}
	if strings.Contains(string(body), "ENCRYPTION_KEY=newkey") {
		t.Errorf("new key overwrote existing: %q", string(body))
	}
}

// Compile-time guard — runInit should accept a non-nil context. We don't
// run the full body in a unit test (it needs a DB), but the function should
// at least be referenceable.
var _ = func() { _ = runInit(context.Background(), initOpts{}) }
