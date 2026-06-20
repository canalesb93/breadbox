//go:build !headless && !lite

package pages

import (
	"testing"

	"breadbox/internal/service"
)

// TestBuildGoverningRule pins the flattening of a service rule response into the
// pure-view components.GoverningRule the detail page's governing-rules panel
// renders: identity carries over, and the condition/action summaries are derived
// via the shared service helpers (so a series' rules read like the /rules list).
func TestBuildGoverningRule(t *testing.T) {
	t.Run("assign_series rule keyed by short_id", func(t *testing.T) {
		resp := service.TransactionRuleResponse{
			ShortID:       "r4Nkr7Sh",
			Name:          "Netflix subscription",
			Conditions:    service.Condition{Field: "merchant", Op: "contains", Value: "netflix"},
			Actions:       []service.RuleAction{{Type: "assign_series", SeriesShortID: "DCSC3LaN"}},
			Enabled:       true,
			HitCount:      4,
			CreatedByType: "user",
			CreatedByName: "Ricardo",
		}
		got := BuildGoverningRule(resp)
		if got.ShortID != "r4Nkr7Sh" || got.Name != "Netflix subscription" {
			t.Fatalf("identity not carried over: %+v", got)
		}
		if got.ConditionSummary == "" {
			t.Errorf("ConditionSummary should be derived, got empty")
		}
		if got.ActionSummary != "Assign to series" {
			t.Errorf("ActionSummary = %q, want %q", got.ActionSummary, "Assign to series")
		}
		if !got.Enabled || got.HitCount != 4 || got.CreatedByType != "user" {
			t.Errorf("scalar fields mismatch: %+v", got)
		}
	})

	t.Run("multi-action rule summarizes the count", func(t *testing.T) {
		resp := service.TransactionRuleResponse{
			ShortID: "PfBp54nN",
			Name:    "Streaming round-up",
			Actions: []service.RuleAction{
				{Type: "assign_series", SeriesName: "Netflix"},
				{Type: "add_tag", TagSlug: "streaming"},
			},
			CreatedByType: "agent",
		}
		got := BuildGoverningRule(resp)
		if got.ActionSummary != "2 actions" {
			t.Errorf("ActionSummary = %q, want %q", got.ActionSummary, "2 actions")
		}
		if got.CreatedByType != "agent" {
			t.Errorf("CreatedByType = %q, want agent", got.CreatedByType)
		}
	})
}

// TestSubscriptionMemberCount pins the row's "N charges" label pluralization.
func TestSubscriptionMemberCount(t *testing.T) {
	cases := map[int]string{0: "0 charges", 1: "1 charge", 4: "4 charges"}
	for n, want := range cases {
		if got := subscriptionMemberCount(n); got != want {
			t.Errorf("subscriptionMemberCount(%d) = %q, want %q", n, got, want)
		}
	}
}

// TestSubscriptionRowMeta pins the body-line composition: charges always show;
// the rule count is appended only when at least one governing rule exists
// (mirrors counterpartyRowMeta so the two surfaces read identically).
func TestSubscriptionRowMeta(t *testing.T) {
	cases := []struct {
		members, rules int
		want           string
	}{
		{0, 0, "0 charges"},
		{1, 0, "1 charge"},
		{4, 1, "4 charges · 1 rule"},
		{4, 3, "4 charges · 3 rules"},
	}
	for _, c := range cases {
		if got := subscriptionRowMeta(c.members, c.rules); got != c.want {
			t.Errorf("subscriptionRowMeta(%d,%d) = %q, want %q", c.members, c.rules, got, c.want)
		}
	}
}

// TestSubscriptionTypeTone pins the type→tone mapping to the vivid tones only
// (so subscription never collapses to gray on the dark theme).
func TestSubscriptionTypeTone(t *testing.T) {
	cases := map[string]string{
		"subscription": "success",
		"bill":         "warning",
		"loan":         "info",
		"other":        "neutral",
		"":             "neutral",
	}
	for typ, want := range cases {
		if got := string(subscriptionTypeTone(typ)); got != want {
			t.Errorf("subscriptionTypeTone(%q) = %q, want %q", typ, got, want)
		}
	}
}
