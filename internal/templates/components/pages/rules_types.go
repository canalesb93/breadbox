package pages

import (
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
		HitCount:         r.HitCount,
		Priority:         r.Priority,
		LastHitAt:        r.LastHitAt,
		CategoryIcon:     r.CategoryIcon,
		CategoryColor:    r.CategoryColor,
		ExpiresAt:        r.ExpiresAt,
		ConditionSummary: service.ConditionSummary(r.Conditions),
		ActionsCount:     len(r.Actions),
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

	return row
}
