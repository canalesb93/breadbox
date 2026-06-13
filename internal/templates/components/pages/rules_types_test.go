//go:build !headless && !lite

package pages

import "testing"

func TestRuleStage(t *testing.T) {
	cases := []struct {
		priority int
		wantSlug string
	}{
		{0, "baseline"},
		{9, "baseline"},
		{10, "standard"},
		{49, "standard"},
		{50, "refinement"},
		{99, "refinement"},
		{100, "override"},
		{1000, "override"},
	}
	for _, c := range cases {
		if slug, _, _ := RuleStage(c.priority); slug != c.wantSlug {
			t.Errorf("RuleStage(%d) = %q, want %q", c.priority, slug, c.wantSlug)
		}
	}
}

func TestGroupRulesByStage(t *testing.T) {
	// Intentionally out of stage order, and with two stages sharing rows,
	// so we exercise both the bucketing and the canonical group ordering.
	rows := []RulesRow{
		{ID: "r1", Priority: 100}, // override
		{ID: "r2", Priority: 10},  // standard
		{ID: "r3", Priority: 0},   // baseline
		{ID: "r4", Priority: 10},  // standard
		{ID: "r5", Priority: 50},  // refinement
		// no second baseline / override — empty buckets must not appear twice
	}

	groups := GroupRulesByStage(rows)
	if len(groups) != 4 {
		t.Fatalf("got %d groups, want 4", len(groups))
	}

	// Groups come back in pipeline-execution order regardless of input order.
	wantOrder := []string{"baseline", "standard", "refinement", "override"}
	for i, want := range wantOrder {
		if groups[i].Slug != want {
			t.Errorf("group[%d] = %q, want %q", i, groups[i].Slug, want)
		}
	}

	// Standard holds both r2 and r4, in their original input order (so the
	// active sort still governs intra-stage ordering).
	std := groups[1]
	if len(std.Rules) != 2 {
		t.Fatalf("standard group: got %d rules, want 2", len(std.Rules))
	}
	if std.Rules[0].ID != "r2" || std.Rules[1].ID != "r4" {
		t.Errorf("standard group order = [%s %s], want [r2 r4]", std.Rules[0].ID, std.Rules[1].ID)
	}

	// Single-rule stages carry exactly their one rule.
	if len(groups[0].Rules) != 1 || groups[0].Rules[0].ID != "r3" {
		t.Errorf("baseline group = %+v, want single r3", groups[0].Rules)
	}
}

func TestGroupRulesByStageEmpty(t *testing.T) {
	if groups := GroupRulesByStage(nil); len(groups) != 0 {
		t.Errorf("GroupRulesByStage(nil) = %d groups, want 0", len(groups))
	}
}
