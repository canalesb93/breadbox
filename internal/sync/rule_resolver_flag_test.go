//go:build !lite

package sync

import (
	"log/slog"
	"testing"
)

// A single flag action sets FlagIntent="flag" and records exactly one source
// keyed on the "flag" action field.
func TestResolveWithContext_Flag(t *testing.T) {
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{{
			id:      testUUID(1),
			shortID: "rflag001",
			name:    "flag big charges",
			actions: []typedAction{{Type: "flag"}},
			trigger: "always",
			condition: mustCompile(t, &Condition{
				Field: "amount", Op: "gt", Value: float64(100),
			}),
		}},
	}
	result := r.ResolveWithContext("plaid", TransactionContext{Name: "Big charge", Amount: 250}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.FlagIntent != "flag" {
		t.Fatalf("expected FlagIntent=flag, got %q", result.FlagIntent)
	}
	flagSources := 0
	for _, s := range result.Sources {
		if s.ActionField == "flag" {
			flagSources++
		}
	}
	if flagSources != 1 {
		t.Errorf("expected 1 flag source, got %d", flagSources)
	}
}

// unflag sets FlagIntent="unflag" with an "unflag" source.
func TestResolveWithContext_Unflag(t *testing.T) {
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{{
			id:        testUUID(1),
			shortID:   "runfl001",
			name:      "clear flag on small",
			actions:   []typedAction{{Type: "unflag"}},
			trigger:   "always",
			condition: mustCompile(t, &Condition{Field: "amount", Op: "lt", Value: float64(5)}),
		}},
	}
	result := r.ResolveWithContext("plaid", TransactionContext{Name: "tiny", Amount: 1}, true)
	if result == nil || result.FlagIntent != "unflag" {
		t.Fatalf("expected FlagIntent=unflag, got %+v", result)
	}
	for _, s := range result.Sources {
		if s.ActionField == "flag" {
			t.Errorf("did not expect a flag source for an unflag-only rule")
		}
	}
}

// Last-writer-wins: a later-stage rule's unflag overrides an earlier flag, and
// only the winning rule's source survives.
func TestResolveWithContext_FlagLastWriterWins(t *testing.T) {
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id: testUUID(1), shortID: "a", name: "A",
				actions:   []typedAction{{Type: "flag"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "x"}),
			},
			{
				id: testUUID(2), shortID: "b", name: "B",
				actions:   []typedAction{{Type: "unflag"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "x"}),
			},
		},
	}
	result := r.ResolveWithContext("plaid", TransactionContext{Name: "x thing"}, true)
	if result.FlagIntent != "unflag" {
		t.Errorf("expected last-writer-wins FlagIntent=unflag, got %q", result.FlagIntent)
	}
	// Exactly one flag-family source survives (the winning unflag).
	flagFamily := 0
	for _, s := range result.Sources {
		if s.ActionField == "flag" || s.ActionField == "unflag" {
			flagFamily++
		}
	}
	if flagFamily != 1 {
		t.Errorf("expected 1 surviving flag-family source, got %d", flagFamily)
	}
}

// parseTypedActions decodes flag / unflag (no params) and tolerates them.
func TestParseTypedActions_Flag(t *testing.T) {
	raw := []byte(`[{"type":"flag"},{"type":"unflag"}]`)
	out := parseTypedActions(raw, testUUID(1), slog.Default())
	if len(out) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(out))
	}
	if out[0].Type != "flag" || out[1].Type != "unflag" {
		t.Errorf("unexpected parsed actions: %+v", out)
	}
}
