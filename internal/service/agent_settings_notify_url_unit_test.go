//go:build !lite

package service

// T17-settings-notify-url: unit tests for the pure (no-DB) logic in
// agent_settings.go that handles NotifyWebhookURL -- validation, masking,
// trimming -- and the related helpers maskToken / lastN / readOptionalFloat.
//
// No DB is required: every function under test is either pure or accepts
// an appconfig.Reader that we satisfy with a lightweight stub.

import (
	"context"
	"errors"
	"testing"

	"breadbox/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Stub appconfig.Reader for readOptionalFloat tests
// ---------------------------------------------------------------------------

// T17fakeReader is a minimal appconfig.Reader that returns a pre-canned value
// for exactly one key or an error when instructed. It satisfies the interface
// GetAppConfig(ctx, key) (db.AppConfig, error).
type T17fakeReader struct {
	key   string
	value string // stored value
	found bool   // if false, simulates a missing row (returns error)
}

func (r *T17fakeReader) GetAppConfig(_ context.Context, key string) (db.AppConfig, error) {
	if !r.found || key != r.key {
		return db.AppConfig{}, errors.New("not found")
	}
	return db.AppConfig{
		Key:   r.key,
		Value: pgtype.Text{String: r.value, Valid: r.value != "" || r.found},
	}, nil
}

// ---------------------------------------------------------------------------
// maskToken
// ---------------------------------------------------------------------------

func TestT17MaskToken_EmptyReturnsNil(t *testing.T) {
	if got := maskToken(""); got != nil {
		t.Errorf("maskToken(\"\") = %q, want nil", *got)
	}
}

func TestT17MaskToken_ShortToken(t *testing.T) {
	// Token <= 20 chars: bullets + last 4 chars
	token := "abc123"
	got := maskToken(token)
	if got == nil {
		t.Fatal("maskToken returned nil for non-empty token")
	}
	// last 4 of "abc123" is "c123"
	want := "\u2022\u2022\u2022\u2022c123"
	if *got != want {
		t.Errorf("maskToken(%q) = %q, want %q", token, *got, want)
	}
}

func TestT17MaskToken_ExactlyFourChars(t *testing.T) {
	token := "abcd"
	got := maskToken(token)
	if got == nil {
		t.Fatal("maskToken returned nil for non-empty token")
	}
	// len("abcd") = 4 <= 20 -- bullets + last 4 = bullets + "abcd"
	want := "\u2022\u2022\u2022\u2022abcd"
	if *got != want {
		t.Errorf("maskToken(%q) = %q, want %q", token, *got, want)
	}
}

func TestT17MaskToken_TwentyCharsExact(t *testing.T) {
	// Exactly 20 chars: short path (<=20): bullets + last 4
	token := "12345678901234567890" // len=20
	got := maskToken(token)
	if got == nil {
		t.Fatal("maskToken returned nil for non-empty token")
	}
	want := "\u2022\u2022\u2022\u20227890"
	if *got != want {
		t.Errorf("maskToken(%q) = %q, want %q", token, *got, want)
	}
}

func TestT17MaskToken_TwentyOneChars(t *testing.T) {
	// Exactly 21 chars: long path (>20): first 16 + bullets + last 4
	token := "123456789012345678901" // len=21
	got := maskToken(token)
	if got == nil {
		t.Fatal("maskToken returned nil for non-empty token")
	}
	// first 16 = "1234567890123456"
	// last 4 of "123456789012345678901" = "8901"
	want := "1234567890123456" + "\u2022\u2022\u2022\u2022" + "8901"
	if *got != want {
		t.Errorf("maskToken(%q) = %q, want %q", token, *got, want)
	}
}

func TestT17MaskToken_LongAPIKey(t *testing.T) {
	// Simulate a realistic Anthropic API key (~60 chars)
	token := "sk-ant-api01-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	got := maskToken(token)
	if got == nil {
		t.Fatal("maskToken returned nil for non-empty token")
	}
	// Must start with first 16 chars of the key
	if (*got)[:16] != token[:16] {
		t.Errorf("maskToken prefix = %q, want %q", (*got)[:16], token[:16])
	}
	// Must end with last 4 chars of the key
	suffix := (*got)[len(*got)-4:]
	wantSuffix := token[len(token)-4:]
	if suffix != wantSuffix {
		t.Errorf("maskToken suffix = %q, want %q", suffix, wantSuffix)
	}
}

func TestT17MaskToken_SubscriptionToken(t *testing.T) {
	// Simulate a Claude subscription token (>20 chars)
	token := "sk-ant-oat01-xyzXYZ9876"
	got := maskToken(token)
	if got == nil {
		t.Fatal("maskToken returned nil for non-empty token")
	}
	// Long path: first16 + bullets + last4
	want := token[:16] + "\u2022\u2022\u2022\u2022" + token[len(token)-4:]
	if *got != want {
		t.Errorf("maskToken(%q) = %q, want %q", token, *got, want)
	}
}

// maskToken must never return the full plaintext token.
func TestT17MaskToken_NeverExposeFullPlaintext(t *testing.T) {
	token := "sk-ant-api01-SUPERSECRET12345678"
	got := maskToken(token)
	if got == nil {
		t.Fatal("unexpected nil")
	}
	if *got == token {
		t.Error("maskToken returned full plaintext token -- must never happen")
	}
}

// ---------------------------------------------------------------------------
// lastN
// ---------------------------------------------------------------------------

func TestT17LastN_Empty(t *testing.T) {
	if got := lastN("", 4); got != "" {
		t.Errorf("lastN(%q, 4) = %q, want %q", "", got, "")
	}
}

func TestT17LastN_ShorterThanN(t *testing.T) {
	if got := lastN("ab", 4); got != "ab" {
		t.Errorf("lastN(%q, 4) = %q, want %q", "ab", got, "ab")
	}
}

func TestT17LastN_EqualToN(t *testing.T) {
	if got := lastN("abcd", 4); got != "abcd" {
		t.Errorf("lastN(%q, 4) = %q, want %q", "abcd", got, "abcd")
	}
}

func TestT17LastN_LongerThanN(t *testing.T) {
	if got := lastN("12345678", 4); got != "5678" {
		t.Errorf("lastN(%q, 4) = %q, want %q", "12345678", got, "5678")
	}
}

// ---------------------------------------------------------------------------
// validateNotifyURL (coverage beyond notify_test.go, focused on the settings
// path which trims whitespace before calling this validator)
// ---------------------------------------------------------------------------

func TestT17ValidateNotifyURL_Valid(t *testing.T) {
	cases := []string{
		"https://ntfy.sh/breadbox",
		"http://localhost:8080/webhook",
		"https://hooks.slack.com/services/T0/B0/xxx",
		"https://discord.com/api/webhooks/123/abc",
		"http://192.168.1.10:9000/hooks/breadbox",
	}
	for _, u := range cases {
		if err := validateNotifyURL(u); err != nil {
			t.Errorf("validateNotifyURL(%q) = %v, want nil", u, err)
		}
	}
}

func TestT17ValidateNotifyURL_Invalid(t *testing.T) {
	cases := []string{
		"",                    // empty
		"ftp://x.com/hook",   // wrong scheme
		"notaurl",             // no scheme
		"javascript:alert(1)", // dangerous scheme
		"https://",            // no host
		"//example.com",       // protocol-relative (no scheme)
		"mailto:x@x.com",     // email URI
	}
	for _, u := range cases {
		if err := validateNotifyURL(u); err == nil {
			t.Errorf("validateNotifyURL(%q) = nil, want error", u)
		}
	}
}

// The settings path trims whitespace before validating. Verify that the
// already-trimmed URL is what gets validated (mirrors the
// trimmed := strings.TrimSpace(*p.NotifyWebhookURL) path in UpdateAgentSettings).
func TestT17ValidateNotifyURL_TrimmedInputs(t *testing.T) {
	cases := []struct {
		name    string
		trimmed string
		wantErr bool
	}{
		{
			name:    "valid https URL passes",
			trimmed: "https://ntfy.sh/topic",
			wantErr: false,
		},
		{
			name:    "slack webhook passes",
			trimmed: "https://hooks.slack.com/services/x",
			wantErr: false,
		},
		// Whitespace-only input collapses to empty after trim.
		// UpdateAgentSettings skips validateNotifyURL when trimmed == "",
		// but the empty string itself must fail if accidentally passed through.
		{
			name:    "empty string (whitespace collapsed) fails",
			trimmed: "",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNotifyURL(tc.trimmed)
			if tc.wantErr && err == nil {
				t.Errorf("validateNotifyURL(%q) = nil, want error", tc.trimmed)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateNotifyURL(%q) = %v, want nil", tc.trimmed, err)
			}
		})
	}
}

// validateNotifyURL must wrap ErrInvalidParameter so callers can use errors.Is.
func TestT17ValidateNotifyURL_WrapsErrInvalidParameter(t *testing.T) {
	err := validateNotifyURL("ftp://bad.example.com")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidParameter) {
		t.Errorf("validateNotifyURL error does not wrap ErrInvalidParameter: %v", err)
	}
}

// ---------------------------------------------------------------------------
// readOptionalFloat -- pure when appconfig.Reader is stubbed
// ---------------------------------------------------------------------------

func TestT17ReadOptionalFloat_MissingKey(t *testing.T) {
	r := &T17fakeReader{key: "x", found: false}
	got := readOptionalFloat(context.Background(), r, "x")
	if got != nil {
		t.Errorf("readOptionalFloat(missing) = %v, want nil", *got)
	}
}

func TestT17ReadOptionalFloat_EmptyValue(t *testing.T) {
	r := &T17fakeReader{key: "x", value: "", found: true}
	got := readOptionalFloat(context.Background(), r, "x")
	if got != nil {
		t.Errorf("readOptionalFloat(empty) = %v, want nil", *got)
	}
}

func TestT17ReadOptionalFloat_ValidFloat(t *testing.T) {
	r := &T17fakeReader{key: "agent.global_max_budget_usd", value: "12.5000", found: true}
	got := readOptionalFloat(context.Background(), r, "agent.global_max_budget_usd")
	if got == nil {
		t.Fatal("readOptionalFloat returned nil, want 12.5")
	}
	if *got != 12.5 {
		t.Errorf("readOptionalFloat = %v, want 12.5", *got)
	}
}

func TestT17ReadOptionalFloat_IntegerValue(t *testing.T) {
	r := &T17fakeReader{key: "k", value: "100", found: true}
	got := readOptionalFloat(context.Background(), r, "k")
	if got == nil {
		t.Fatal("readOptionalFloat returned nil, want 100")
	}
	if *got != 100 {
		t.Errorf("readOptionalFloat = %v, want 100", *got)
	}
}

func TestT17ReadOptionalFloat_ZeroValue(t *testing.T) {
	r := &T17fakeReader{key: "k", value: "0", found: true}
	got := readOptionalFloat(context.Background(), r, "k")
	if got == nil {
		t.Fatal("readOptionalFloat returned nil, want 0")
	}
	if *got != 0 {
		t.Errorf("readOptionalFloat = %v, want 0", *got)
	}
}

func TestT17ReadOptionalFloat_UnparseableValue(t *testing.T) {
	r := &T17fakeReader{key: "k", value: "not-a-float", found: true}
	got := readOptionalFloat(context.Background(), r, "k")
	if got != nil {
		t.Errorf("readOptionalFloat(unparseable) = %v, want nil", *got)
	}
}

func TestT17ReadOptionalFloat_WrongKey(t *testing.T) {
	// Reader has key "a" but we ask for "b" -- should return nil.
	r := &T17fakeReader{key: "a", value: "5.5", found: true}
	got := readOptionalFloat(context.Background(), r, "b")
	if got != nil {
		t.Errorf("readOptionalFloat(wrong key) = %v, want nil", *got)
	}
}

// ---------------------------------------------------------------------------
// appconfigParam helper
// ---------------------------------------------------------------------------

func TestT17AppconfigParam_BuildsCorrectly(t *testing.T) {
	p := appconfigParam("notify.webhook_url", "https://ntfy.sh/test")
	if p.Key != "notify.webhook_url" {
		t.Errorf("Key = %q, want %q", p.Key, "notify.webhook_url")
	}
	if !p.Value.Valid {
		t.Error("Value.Valid = false, want true")
	}
	if p.Value.String != "https://ntfy.sh/test" {
		t.Errorf("Value.String = %q, want %q", p.Value.String, "https://ntfy.sh/test")
	}
}

func TestT17AppconfigParam_EmptyValue(t *testing.T) {
	// Clearing a webhook URL stores an empty string (notifications off).
	p := appconfigParam("notify.webhook_url", "")
	if !p.Value.Valid {
		t.Error("Value.Valid = false, want true even for empty string")
	}
	if p.Value.String != "" {
		t.Errorf("Value.String = %q, want empty", p.Value.String)
	}
}
