//go:build !headless && !lite

package admin

import "testing"

// TestAgentSlugFromKeyName covers the parse that lets agent-authored
// activity (whose actor_id is the per-run api_key UUID) resolve back to
// the agent's stable slug, so every run of an agent shares one avatar
// matching its /agents/<slug> profile. Only the minted run-key shape
// "agent:<slug>:<runID>" should match; anything else falls back to
// seeding on the key UUID.
func TestAgentSlugFromKeyName(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantSlug string
		wantOK   bool
	}{
		{"minted run key", "agent:daily-triage:r4nd0m42", "daily-triage", true},
		{"single-char slug", "agent:x:abc123de", "x", true},
		{"hyphenated slug", "agent:sync-improve:run1234", "sync-improve", true},
		{"empty slug segment", "agent::run1234", "", false},
		{"wrong prefix", "mcp:daily-triage:run1234", "", false},
		{"too few segments", "agent:daily-triage", "", false},
		{"too many segments", "agent:daily-triage:run:extra", "", false},
		{"bare uuid (http mcp key)", "d290f1ee-6c54-4b01-90e6-d701748f0851", "", false},
		{"empty string", "", "", false},
		{"human key name", "My laptop key", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotSlug, gotOK := agentSlugFromKeyName(tc.in)
			if gotOK != tc.wantOK || gotSlug != tc.wantSlug {
				t.Fatalf("agentSlugFromKeyName(%q) = (%q, %v), want (%q, %v)",
					tc.in, gotSlug, gotOK, tc.wantSlug, tc.wantOK)
			}
		})
	}
}
