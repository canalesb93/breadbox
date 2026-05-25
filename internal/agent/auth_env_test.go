//go:build !lite

package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAuthEnvFor(t *testing.T) {
	const apiToken = "sk-ant-test-apikey"
	const subToken = "sk-ant-oat01-test-subscription"

	inherited := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_API_KEY=stale-from-shell",
		"CLAUDE_CODE_OAUTH_TOKEN=stale-from-shell",
		"BREADBOX_AGENT_TRANSCRIPT_DIR=/tmp/x",
	}

	cases := []struct {
		name      string
		auth      AuthConfig
		wantSet   string // env var that should be present
		wantValue string
		wantUnset string // the OTHER auth var must be absent
	}{
		{
			name:      "api_key mode",
			auth:      AuthConfig{Mode: "api_key", Token: apiToken},
			wantSet:   "ANTHROPIC_API_KEY",
			wantValue: apiToken,
			wantUnset: "CLAUDE_CODE_OAUTH_TOKEN",
		},
		{
			name:      "subscription mode",
			auth:      AuthConfig{Mode: "subscription", Token: subToken},
			wantSet:   "CLAUDE_CODE_OAUTH_TOKEN",
			wantValue: subToken,
			wantUnset: "ANTHROPIC_API_KEY",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := authEnvFor(inherited, tc.auth)

			// Both stale entries must be stripped.
			for _, e := range got {
				if strings.HasPrefix(e, "ANTHROPIC_API_KEY=stale") ||
					strings.HasPrefix(e, "CLAUDE_CODE_OAUTH_TOKEN=stale") {
					t.Errorf("stale auth env survived: %q", e)
				}
			}
			// Non-auth inherited vars are preserved.
			if !contains(got, "PATH=/usr/bin") {
				t.Errorf("PATH=/usr/bin missing from result: %v", got)
			}
			if !contains(got, "BREADBOX_AGENT_TRANSCRIPT_DIR=/tmp/x") {
				t.Errorf("BREADBOX_AGENT_TRANSCRIPT_DIR missing from result: %v", got)
			}
			// The active var is set, and only once.
			want := tc.wantSet + "=" + tc.wantValue
			if !contains(got, want) {
				t.Errorf("expected %q in result, got %v", want, got)
			}
			// The inactive var is absent.
			for _, e := range got {
				if strings.HasPrefix(e, tc.wantUnset+"=") {
					t.Errorf("expected %q to be unset, found %q", tc.wantUnset, e)
				}
			}
		})
	}
}

func TestAuthEnvFor_EmptyToken(t *testing.T) {
	// Empty token (e.g. unit tests, pre-auth smoke) must not lay down an empty
	// env value — neither var should be set so the SDK fails with its own
	// clear "no auth" message instead of "auth header malformed".
	got := authEnvFor([]string{"PATH=/usr/bin"}, AuthConfig{Mode: "api_key", Token: ""})
	for _, e := range got {
		if strings.HasPrefix(e, "ANTHROPIC_API_KEY=") || strings.HasPrefix(e, "CLAUDE_CODE_OAUTH_TOKEN=") {
			t.Errorf("empty token should not produce auth env, got %q", e)
		}
	}
}

func TestAuthConfig_JSON_OmitsToken(t *testing.T) {
	auth := AuthConfig{Mode: "subscription", Token: "sk-ant-oat01-supersecret"}
	b, err := json.Marshal(auth)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "supersecret") {
		t.Errorf("plaintext token leaked into JSON wire format: %s", b)
	}
	if !strings.Contains(string(b), `"mode":"subscription"`) {
		t.Errorf("mode missing from JSON: %s", b)
	}
}

func TestAuthConfig_String_Redacts(t *testing.T) {
	auth := AuthConfig{Mode: "api_key", Token: "sk-ant-api03-abcdef1234567890"}
	s := auth.String()
	if strings.Contains(s, "abcdef1234567890") {
		t.Errorf("String() leaked full token: %s", s)
	}
	// Last 4 chars are fine for identifying which token is in use during debugging.
	if !strings.Contains(s, "7890") {
		t.Errorf("String() should include last-4 for identification, got %q", s)
	}
	// %v / %+v / %#v should all go through String()/GoString().
	if got := fmt.Sprintf("%v", auth); strings.Contains(got, "abcdef1234567890") {
		t.Errorf("%%v leaked full token: %s", got)
	}
	if got := fmt.Sprintf("%#v", auth); strings.Contains(got, "abcdef1234567890") {
		t.Errorf("%%#v leaked full token: %s", got)
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
