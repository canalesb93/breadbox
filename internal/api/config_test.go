package api

import (
	"strings"
	"testing"
)

func TestIsSecretKey(t *testing.T) {
	t.Helper()
	cases := []struct {
		key  string
		want bool
	}{
		{"plaid_client_id", false},
		{"plaid_secret", true},
		{"plaid_env", false},
		{"webhook_url", false},
		{"teller_app_id", false},
		{"teller_webhook_secret", true},
		{"teller_cert_pem", false}, // not in the substring list, but blocked by isAlwaysDenied
		{"teller_key_pem", false},  // blocked by isAlwaysDenied; not flagged by suffix
		{"some_api_key", true},     // ends in _KEY → secret
		{"ENCRYPTION_KEY", true},
		{"api_token", true},
		{"some_password", true},
	}
	for _, c := range cases {
		got := isSecretKey(c.key)
		if got != c.want {
			t.Errorf("isSecretKey(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}

func TestIsAlwaysDenied(t *testing.T) {
	if !isAlwaysDenied("ENCRYPTION_KEY") {
		t.Error("ENCRYPTION_KEY should be always denied")
	}
	if !isAlwaysDenied("teller_cert_pem") {
		t.Error("teller_cert_pem should be always denied")
	}
	if isAlwaysDenied("plaid_client_id") {
		t.Error("plaid_client_id should not be always denied")
	}
}

func TestMaskValue(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"abc", "****"},
		{"abcd", "****"},
		{"bb_xx", "***xx"},
		{"sandbox12", "*******12"},
		{"long-value-here-1234567890", "long********..."},
	}
	for _, c := range cases {
		got := maskValue(c.in)
		if got != c.want {
			t.Errorf("maskValue(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// Mask must never leak the full value when input is "secret-shaped".
	masked := maskValue("very-long-actual-secret-string")
	if strings.Contains(masked, "secret-string") {
		t.Errorf("maskValue leaked tail: %q", masked)
	}
}
