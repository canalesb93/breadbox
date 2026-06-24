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
