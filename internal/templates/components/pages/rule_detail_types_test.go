//go:build !headless && !lite

package pages

import (
	"testing"

	"breadbox/internal/service"
)

// retroactiveActionTypes is the canonical set of action types that
// Service.ApplyRuleRetroactively materializes against already-synced
// transactions (internal/service/rules.go). add_comment is deliberately
// excluded — it is the only sync-only action. This list is the source of
// truth the UI gate (ruleHasRetroactiveAction) must mirror so the rule
// detail page never hides "Apply now" for a rule the engine would back-fill.
var retroactiveActionTypes = []string{
	"set_category",
	"add_tag",
	"remove_tag",
	"assign_series",
	"assign_counterparty",
	"set_metadata",
	"remove_metadata",
	"flag",
	"unflag",
}

func TestRuleHasRetroactiveActionMatchesEngineSet(t *testing.T) {
	for _, typ := range retroactiveActionTypes {
		actions := []service.RuleAction{{Type: typ}}
		if !ruleHasRetroactiveAction(actions) {
			t.Errorf("ruleHasRetroactiveAction(%q) = false, want true — the engine materializes this action retroactively", typ)
		}
	}
}

func TestRuleHasRetroactiveActionExcludesCommentOnly(t *testing.T) {
	// add_comment is sync-only; a comment-only rule must NOT surface the
	// retroactive Apply affordance.
	if ruleHasRetroactiveAction([]service.RuleAction{{Type: "add_comment"}}) {
		t.Error("ruleHasRetroactiveAction([add_comment]) = true, want false — add_comment is sync-only")
	}
	// An empty action list is also non-retroactive.
	if ruleHasRetroactiveAction(nil) {
		t.Error("ruleHasRetroactiveAction(nil) = true, want false")
	}
}

func TestRuleActionEntityNamePrefersInlineName(t *testing.T) {
	// An action that carries an inline name renders it verbatim, regardless of
	// any resolved-name map.
	p := RuleDetailProps{ActionEntityNames: map[string]string{"wvmXi45D": "Resolved"}}
	cp := service.RuleAction{Type: "assign_counterparty", CounterpartyName: "Netflix", CounterpartyShortID: "wvmXi45D"}
	if got := ruleActionEntityName(p, cp); got != "Netflix" {
		t.Errorf("ruleActionEntityName(counterparty inline) = %q, want Netflix", got)
	}
	sr := service.RuleAction{Type: "assign_series", SeriesName: "Hulu", SeriesShortID: "abcd1234"}
	if got := ruleActionEntityName(p, sr); got != "Hulu" {
		t.Errorf("ruleActionEntityName(series inline) = %q, want Hulu", got)
	}
}

func TestRuleActionEntityNameResolvesShortID(t *testing.T) {
	// The bug from #1916: a rule that binds an existing counterparty by short_id
	// stores no inline name, so the card used to render the opaque surrogate.
	// With the handler-hydrated ActionEntityNames map, the short_id resolves to
	// the entity's display name.
	p := RuleDetailProps{ActionEntityNames: map[string]string{
		"wvmXi45D": "Netflix",
		"abcd1234": "Spotify",
	}}
	cp := service.RuleAction{Type: "assign_counterparty", CounterpartyShortID: "wvmXi45D"}
	if got := ruleActionEntityName(p, cp); got != "Netflix" {
		t.Errorf("ruleActionEntityName(counterparty short_id) = %q, want Netflix", got)
	}
	sr := service.RuleAction{Type: "assign_series", SeriesShortID: "abcd1234"}
	if got := ruleActionEntityName(p, sr); got != "Spotify" {
		t.Errorf("ruleActionEntityName(series short_id) = %q, want Spotify", got)
	}
}

func TestRuleActionEntityNameFallsBackToShortID(t *testing.T) {
	// When the lookup misses (entity deleted, resolution failed), the short_id
	// is the last-resort display value rather than an empty label.
	p := RuleDetailProps{ActionEntityNames: map[string]string{}}
	cp := service.RuleAction{Type: "assign_counterparty", CounterpartyShortID: "wvmXi45D"}
	if got := ruleActionEntityName(p, cp); got != "wvmXi45D" {
		t.Errorf("ruleActionEntityName(unresolved counterparty) = %q, want wvmXi45D", got)
	}
}

func TestRuleHasRetroactiveActionMixedRetroWins(t *testing.T) {
	// A rule that mixes a sync-only action with a materializing one is still
	// retroactive — the materializing action gets backfilled.
	actions := []service.RuleAction{
		{Type: "add_comment"},
		{Type: "assign_counterparty"},
	}
	if !ruleHasRetroactiveAction(actions) {
		t.Error("ruleHasRetroactiveAction([add_comment, assign_counterparty]) = false, want true")
	}
}
