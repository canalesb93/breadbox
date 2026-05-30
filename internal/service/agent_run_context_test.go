//go:build !lite

package service

import (
	"context"
	"testing"

	"breadbox/internal/db"
)

func TestParseAgentKeySlug(t *testing.T) {
	cases := []struct {
		name, in, want string
		ok             bool
	}{
		{"run key", "agent:review-agent:Run12345", "review-agent", true},
		{"hyphenated slug", "agent:sync-improve:abc", "sync-improve", true},
		{"empty slug", "agent::Run1", "", false},
		{"mcp-client key", "mcp-client:claude_code@@stdio", "", false},
		{"too few parts", "agent:review-agent", "", false},
		{"too many parts", "agent:review:agent:run", "", false},
		{"human key name", "My laptop key", "", false},
		{"empty", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseAgentKeySlug(tc.in)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("ParseAgentKeySlug(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// TestIsAgentRunContext covers the rebind gate. The actor_type='agent'
// check is load-bearing: a non-agent key merely NAMED like a run key
// must NOT be treated as a run (otherwise an operator could create a
// full_access user key named "agent:..." and suppress the clientInfo
// rebind to spoof an agent identity).
func TestIsAgentRunContext(t *testing.T) {
	mk := func(actorType, name string) context.Context {
		return ContextWithAPIKey(context.Background(), &db.ApiKey{
			ID: mustUUID(0x11), Name: name, ActorType: actorType,
		})
	}
	cases := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{"real run key", mk("agent", "agent:review-agent:Run1"), true},
		{"spoofed user key named agent:*", mk("user", "agent:review-agent:Run1"), false},
		{"local mcp fallback (agent type, mcp-client name)", mk("agent", "mcp-client:claude_code@@stdio"), false},
		{"agent key, non-run name", mk("agent", "some-other-agent-key"), false},
		{"no key in ctx", context.Background(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAgentRunContext(tc.ctx); got != tc.want {
				t.Fatalf("IsAgentRunContext = %v, want %v", got, tc.want)
			}
		})
	}
}
