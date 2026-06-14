//go:build !headless && !lite

package pages

import (
	"sort"

	"breadbox/internal/service"
)

// RulesProps mirrors the data map the old rules.html read off the layout.
// Built once by admin/rules.go and passed straight to the templ — keeps
// pgtype out of the template and lets the handler decide which fields to
// pre-format (relative time for LastHitAt).
type RulesProps struct {
	Rules        []RulesRow
	Total        int64
	Page         int
	PageSize     int
	TotalPages   int
	ShowingStart int
	ShowingEnd   int64

	// Pre-built "/rules?…&page=" prefix; pagination links append the page
	// number directly. Mirrors the existing template var.
	PaginationBase string

	// Sort key drives the <select> in the toolbar. Empty == default
	// (priority).
	SortBy string

	// Filter state — driven by query params, echoed back so the toolbar
	// shows the active filter. Empty string == no filter for that dim.
	Search       string
	CategorySlug string
	EnabledFilter string // "", "true", "false"
	CreatorType   string // "", "user", "agent", "system"

	// FiltersActive is true when any non-default filter is set; the
	// toolbar uses it to show a "Clear" link.
	FiltersActive bool

	// FilterCategories is the flat list of categories used to populate
	// the category-filter <select>. Built by the handler from
	// ListCategoryTree to avoid a service call inside the templ.
	FilterCategories []RulesCategoryOption
}

// RulesCategoryOption is a flattened, display-ready entry for the
// category filter <select>. Indented children are pre-formatted in the
// handler so the templ stays free of tree traversal.
type RulesCategoryOption struct {
	Slug  string
	Label string
}

// RulesRow is a flat view-model for one rule card. Pre-renders the bits
// that need helper logic (relative time, condition counts, expired flag)
// so the templ stays free of funcMap shims.
type RulesRow struct {
	ID         string
	Name       string
	Enabled    bool
	IsSystem   bool
	HitCount   int
	Priority   int
	LastHitAt  *string // RFC3339; rendered via LastHitAtRelative
	LastHitAtRelative string

	// Creator identity — drives the agent avatar on agent-authored rows
	// (principle 7). CreatedByType is one of user / agent / system;
	// IsSystem is the pre-derived convenience flag for the System badge.
	CreatedByType string
	CreatedByID   string
	CreatedByName string

	CategoryIcon  *string
	CategoryColor *string

	ExpiresAt *string
	Expired   bool

	// Conditions/actions surface counts + summaries; the row only needs
	// the scalar derivatives.
	ConditionCount  int
	IsMatchAll      bool
	ConditionSummary string
	ActionsCount    int
	ActionsSummary  string
	// PrimaryActionType is the single action's type ("set_category",
	// "add_tag", "remove_tag", "add_comment", "assign_series") when the
	// rule has exactly one action; "" when it has zero or multiple actions.
	// Drives the Action-column icon so the column reflects what the rule
	// *does*, not just whether it sets a category.
	PrimaryActionType string
}

// BuildRulesRow flattens a service.TransactionRuleResponse into the
// flat view-model the rule card needs. Centralizes the derivations that
// used to live in funcMap (isMatchAllCondition, conditionCount,
// actionsSummary, expired). Caller fills LastHitAtRelative + Expired
// from the admin handler so this package stays free of time helpers.
func BuildRulesRow(r service.TransactionRuleResponse) RulesRow {
	row := RulesRow{
		ID:               r.ID,
		Name:             r.Name,
		Enabled:          r.Enabled,
		IsSystem:         r.CreatedByType == "system",
		CreatedByType:    r.CreatedByType,
		CreatedByName:    r.CreatedByName,
		HitCount:         r.HitCount,
		Priority:         r.Priority,
		LastHitAt:        r.LastHitAt,
		CategoryIcon:     r.CategoryIcon,
		CategoryColor:    r.CategoryColor,
		ExpiresAt:        r.ExpiresAt,
		ConditionSummary: service.ConditionSummary(r.Conditions),
		ActionsCount:     len(r.Actions),
	}
	if r.CreatedByID != nil {
		row.CreatedByID = *r.CreatedByID
	}

	row.IsMatchAll = r.Conditions.Field == "" && len(r.Conditions.And) == 0 && len(r.Conditions.Or) == 0 && r.Conditions.Not == nil
	switch {
	case len(r.Conditions.And) > 0:
		row.ConditionCount = len(r.Conditions.And)
	case len(r.Conditions.Or) > 0:
		row.ConditionCount = len(r.Conditions.Or)
	case r.Conditions.Field != "":
		row.ConditionCount = 1
	}

	categoryName := ""
	if r.CategoryName != nil {
		categoryName = *r.CategoryName
	}
	row.ActionsSummary = service.ActionsSummary(r.Actions, categoryName)
	if len(r.Actions) == 1 {
		row.PrimaryActionType = r.Actions[0].Type
	}

	return row
}

// RulesStageGroup is one pipeline-stage bucket of rules, the grouping axis
// for the /rules surface. Rules execute in priority order during sync, so
// grouping by stage makes the page read as the pipeline it drives. Order
// is the canonical stage index (0=baseline … 3=override) — the pipeline's
// execution order, top to bottom.
type RulesStageGroup struct {
	Slug        string
	Label       string
	Description string
	Order       int
	Rules       []RulesRow
}

// RuleStage maps a rule's numeric pipeline priority to its canonical stage
// bucket (slug, label, one-line description). The DSL names four stages —
// baseline(0) / standard(10) / refinement(50) / override(100) — but
// `priority` is a free 0..1000 integer, so each named stage owns the
// half-open range up to the next one. Kept in lock-step with
// docs/rule-dsl.md → "Priority as pipeline stage".
func RuleStage(priority int) (slug, label, desc string) {
	switch {
	case priority < 10:
		return "baseline", "Baseline", "Foundation — broad, early classifications"
	case priority < 50:
		return "standard", "Standard", "The default rule stage"
	case priority < 100:
		return "refinement", "Refinement", "Reacts to baseline & standard output"
	default:
		return "override", "Override", "Final say — runs last"
	}
}

// ruleStageOrder is the canonical execution order of the named stages,
// driving both the group sort and RulesStageGroup.Order.
var ruleStageOrder = map[string]int{
	"baseline":   0,
	"standard":   1,
	"refinement": 2,
	"override":   3,
}

// GroupRulesByStage buckets rows into their pipeline stage, preserving the
// caller's row order within each bucket (so the active sort still governs
// intra-stage ordering) and returning the groups in pipeline-execution
// order (baseline → override). Empty stages are omitted. Pure and
// order-stable so the IA decision is pinned by a unit test rather than
// vibes (mirrors GroupAccountsByConnection).
func GroupRulesByStage(rows []RulesRow) []RulesStageGroup {
	bySlug := make(map[string]*RulesStageGroup)
	for _, r := range rows {
		slug, label, desc := RuleStage(r.Priority)
		g, ok := bySlug[slug]
		if !ok {
			g = &RulesStageGroup{Slug: slug, Label: label, Description: desc, Order: ruleStageOrder[slug]}
			bySlug[slug] = g
		}
		g.Rules = append(g.Rules, r)
	}
	out := make([]RulesStageGroup, 0, len(bySlug))
	for _, g := range bySlug {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}
