//go:build !headless && !lite

package pages

// design_rules_list_helpers.go holds the fixture rows for the
// SectionRulesList sandbox entry. Kept beside the design types so the
// templ stays focused on layout. The variants exercise every axis of the
// /rules surface: the four pipeline stages (the grouping), enabled vs
// disabled (the toggle / row dim), system vs agent vs user creators (the
// System badge + agent avatar), an expired rule, and a never-hit rule.

func lastHit(s string) *string { v := s; return &v }

// designRuleRows returns the flat fixture matrix; the sandbox groups it
// through GroupRulesByStage so the live grouping logic is what renders.
func designRuleRows() []RulesRow {
	return []RulesRow{
		{
			ID: "rule_base01", Name: "Income → Salary", Enabled: true,
			Priority: 0, CreatedByType: "system", IsSystem: true,
			HitCount: 412, LastHitAt: lastHit("x"), LastHitAtRelative: "2h ago",
			PrimaryActionType: "set_category", ActionsSummary: "Set category Salary",
		},
		{
			ID: "rule_std01", Name: "Starbucks → Coffee", Enabled: true,
			Priority: 10, CreatedByType: "user",
			HitCount: 96, LastHitAt: lastHit("x"), LastHitAtRelative: "1d ago",
			PrimaryActionType: "set_category", ActionsSummary: "Set category Coffee",
		},
		{
			ID: "rule_std02", Name: "Tag all Amazon orders", Enabled: false,
			Priority: 10, CreatedByType: "user",
			HitCount: 0, LastHitAtRelative: "",
			PrimaryActionType: "add_tag", ActionsSummary: "Add tag shopping",
		},
		{
			ID: "rule_ref01", Name: "Coffee runs → Dining tag", Enabled: true,
			Priority: 50, CreatedByType: "agent", CreatedByID: "agent_curator",
			CreatedByName: "Category curator",
			HitCount: 38, LastHitAt: lastHit("x"), LastHitAtRelative: "4h ago",
			PrimaryActionType: "add_tag", ActionsSummary: "Add tag dining",
		},
		{
			ID: "rule_ref02", Name: "Flag large unknowns", Enabled: true,
			Priority: 50, CreatedByType: "agent", CreatedByID: "agent_sentry",
			CreatedByName: "Fraud sentry", ExpiresAt: lastHit("x"), Expired: true,
			HitCount: 5, LastHitAt: lastHit("x"), LastHitAtRelative: "6d ago",
			PrimaryActionType: "add_comment", ActionsSummary: "Add comment \"review me\"",
		},
		{
			ID: "rule_ovr01", Name: "Manual override: rent", Enabled: true,
			Priority: 100, CreatedByType: "user",
			HitCount: 12, LastHitAt: lastHit("x"), LastHitAtRelative: "3d ago",
			PrimaryActionType: "set_category", ActionsSummary: "Set category Rent",
		},
	}
}

// designRuleGroups runs the fixtures through the real grouping helper so
// the sandbox renders exactly what the live page does.
func designRuleGroups() []RulesStageGroup {
	return GroupRulesByStage(designRuleRows())
}
