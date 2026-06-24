//go:build !lite

package sync

import (
	"log/slog"
	"testing"
)

// parseTypedActions decodes assign_counterparty by short_id and by name.
func TestParseTypedActions_AssignCounterparty(t *testing.T) {
	raw := []byte(`[
		{"type":"assign_counterparty","counterparty_short_id":"cp123abc"},
		{"type":"assign_counterparty","counterparty_name":"Venmo","create_if_missing":true}
	]`)
	out := parseTypedActions(raw, testUUID(1), slog.Default())
	if len(out) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(out))
	}
	if out[0].Type != "assign_counterparty" || out[0].CounterpartyShortID != "cp123abc" {
		t.Errorf("short_id action mis-parsed: %+v", out[0])
	}
	if out[1].CounterpartyName != "Venmo" || !out[1].CreateIfMissing {
		t.Errorf("name action mis-parsed: %+v", out[1])
	}
}

// ResolveWithContext records a CounterpartyAssign intent and one counterparty
// source for a matching assign_counterparty rule.
func TestResolveWithContext_AssignCounterparty(t *testing.T) {
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(10),
				shortID:   "rulecp01",
				name:      "Venmo → counterparty",
				actions:   []typedAction{{Type: "assign_counterparty", CounterpartyName: "Venmo", CreateIfMissing: true}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "venmo"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	result := r.ResolveWithContext("plaid", TransactionContext{Name: "VENMO PAYMENT"}, true)
	if result == nil || result.CounterpartyAssign == nil {
		t.Fatal("expected a CounterpartyAssign intent")
	}
	if result.CounterpartyAssign.CounterpartyName != "Venmo" || !result.CounterpartyAssign.CreateIfMissing {
		t.Errorf("unexpected intent: %+v", result.CounterpartyAssign)
	}
	cpSources := 0
	for _, s := range result.Sources {
		if s.ActionField == "counterparty" {
			cpSources++
			if s.ActionValue != "Venmo" {
				t.Errorf("counterparty source value = %q, want Venmo", s.ActionValue)
			}
		}
	}
	if cpSources != 1 {
		t.Errorf("expected 1 counterparty source, got %d", cpSources)
	}
}

// ResolveWithContext is last-writer-wins for assign_counterparty: a later-stage
// rule overrides the earlier binding and supersedes its audit source.
func TestResolveWithContext_AssignCounterpartyLastWriterWins(t *testing.T) {
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(1),
				shortID:   "rulelo01",
				name:      "low",
				actions:   []typedAction{{Type: "assign_counterparty", CounterpartyShortID: "cpAAAAAA"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "x"}),
			},
			{
				id:        testUUID(2),
				shortID:   "rulehi01",
				name:      "high",
				actions:   []typedAction{{Type: "assign_counterparty", CounterpartyShortID: "cpBBBBBB"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "x"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	result := r.ResolveWithContext("plaid", TransactionContext{Name: "x thing"}, true)
	if result == nil || result.CounterpartyAssign == nil {
		t.Fatal("expected a CounterpartyAssign intent")
	}
	if result.CounterpartyAssign.CounterpartyShortID != "cpBBBBBB" {
		t.Errorf("last-writer-wins failed: got %q, want cpBBBBBB", result.CounterpartyAssign.CounterpartyShortID)
	}
	cpSources := 0
	for _, s := range result.Sources {
		if s.ActionField == "counterparty" {
			cpSources++
			if s.ActionValue != "cpBBBBBB" {
				t.Errorf("surviving source value = %q, want cpBBBBBB", s.ActionValue)
			}
		}
	}
	if cpSources != 1 {
		t.Errorf("expected exactly 1 surviving counterparty source, got %d", cpSources)
	}
}

// evaluateLeaf matches the counterparty (short_id) and has_counterparty (bool)
// condition fields.
func TestEvaluateLeaf_Counterparty(t *testing.T) {
	tctx := TransactionContext{CounterpartyShortID: "cp123abc", HasCounterparty: true}

	yes := mustCompile(t, &Condition{Field: "counterparty", Op: "eq", Value: "cp123abc"})
	if !evaluateCondition(yes, tctx) {
		t.Error("expected counterparty eq match")
	}
	no := mustCompile(t, &Condition{Field: "counterparty", Op: "eq", Value: "cpZZZZZZ"})
	if evaluateCondition(no, tctx) {
		t.Error("expected counterparty eq non-match")
	}
	has := mustCompile(t, &Condition{Field: "has_counterparty", Op: "eq", Value: true})
	if !evaluateCondition(has, tctx) {
		t.Error("expected has_counterparty true match")
	}
	hasNot := mustCompile(t, &Condition{Field: "has_counterparty", Op: "eq", Value: true})
	if evaluateCondition(hasNot, TransactionContext{}) {
		t.Error("expected has_counterparty true to NOT match an unbound transaction")
	}
}
