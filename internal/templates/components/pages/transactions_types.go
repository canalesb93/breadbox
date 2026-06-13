//go:build !headless && !lite

package pages

import (
	"time"

	"breadbox/internal/service"
	"breadbox/internal/templates/components"
)

// txDatePreset is one quick date-range chip in the "When" filter section.
// Start/End are pre-formatted YYYY-MM-DD strings (the value= shape a native
// <input type="date"> expects) so the Alpine click handler only assigns them
// to the two date inputs — no client-side date math.
type txDatePreset struct {
	Label string
	Start string
	End   string
}

// txDatePresets returns the canonical quick date ranges, computed server-side
// relative to now: This month, Last 30 days, Last 90 days, This year. Each
// fills both date inputs in the When section on click.
func txDatePresets() []txDatePreset {
	const iso = "2006-01-02"
	now := time.Now()
	today := now.Format(iso)
	return []txDatePreset{
		{
			Label: "This month",
			Start: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format(iso),
			End:   today,
		},
		{
			Label: "Last 30 days",
			Start: now.AddDate(0, 0, -29).Format(iso),
			End:   today,
		},
		{
			Label: "Last 90 days",
			Start: now.AddDate(0, 0, -89).Format(iso),
			End:   today,
		},
		{
			Label: "This year",
			Start: time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()).Format(iso),
			End:   today,
		},
	}
}

// TransactionsConnectionOption represents one entry in the Connection filter dropdown.
type TransactionsConnectionOption struct {
	ID              string
	InstitutionName string
}

// TransactionsAccountOption represents one entry in the Account filter dropdown.
type TransactionsAccountOption struct {
	ID   string
	Name string
	Mask string
}

// TransactionsUserOption represents one entry in the Family Member filter dropdown.
type TransactionsUserOption struct {
	ID   string
	Name string
}

// TransactionsFilterChip describes one active-filter chip rendered above the
// transaction list when the filter panel is collapsed. Each chip shows a
// human-readable label and links to the current URL with just that one
// filter dropped so the user can clear filters individually without having
// to open the panel.
type TransactionsFilterChip struct {
	Label     string
	RemoveURL string
}

// TransactionsProps is the full view model the transactions page renders.
// Hosted inside base.html via TemplateRenderer.RenderWithTempl. The AJAX
// tx-results fragment (TxResultsProps) is embedded so the same templ
// component drives both the initial render and the fetch swap.
type TransactionsProps struct {
	CSRFToken string

	// Totals & data
	Total        int64
	Transactions []service.AdminTransactionRow

	// Filter dropdown options.
	Connections []TransactionsConnectionOption
	Accounts    []TransactionsAccountOption
	Users       []TransactionsUserOption
	Categories  []service.CategoryResponse
	AllTags     []service.TagResponse

	// Active filter values (string form, ready for value= attrs).
	FilterStartDate   string
	FilterEndDate     string
	FilterAccountID   string
	FilterUserID      string
	FilterConnID      string
	FilterCategory    string
	FilterMinAmount   string
	FilterMaxAmount   string
	FilterPending     string
	FilterFlagged     string
	FilterSearch      string
	FilterSearchMode  string
	FilterSearchField string
	FilterSort        string
	FilterTags        []string
	FilterAnyTag      []string

	// Active-filter chip summary. Populated by the admin handler so the view
	// can render one chip per applied filter when the panel is collapsed.
	FilterChips []TransactionsFilterChip

	// Export + pagination base URLs.
	ExportURL string

	// Results/pagination (mirrors TxResultsProps so the same fragment can be
	// rendered inline and served standalone for the AJAX swap path).
	Results components.TxResultsProps
}

// HasActiveFilters reports whether any non-search filter is currently applied.
// Mirrors the mixed {{if or .FilterStartDate ...}} check in the original
// transactions.html. Kept on the props so both the templ component and tests
// can reason about the flag the same way.
func (p TransactionsProps) HasActiveFilters() bool {
	return p.FilterStartDate != "" ||
		p.FilterEndDate != "" ||
		p.FilterAccountID != "" ||
		p.FilterUserID != "" ||
		p.FilterConnID != "" ||
		p.FilterCategory != "" ||
		p.FilterMinAmount != "" ||
		p.FilterMaxAmount != "" ||
		p.FilterPending != "" ||
		p.FilterFlagged != "" ||
		len(p.FilterTags) > 0 ||
		len(p.FilterAnyTag) > 0
}

// HasAnyFilter returns HasActiveFilters OR an active search. The filter-panel
// "Active" badge and the empty-state "Clear filters" link use this broader
// check; the filtersOpen auto-expand uses only HasActiveFilters (search has
// its own visible bar).
func (p TransactionsProps) HasAnyFilter() bool {
	return p.HasActiveFilters() || p.FilterSearch != ""
}

// ── Filter-drawer section smart-defaults ─────────────────────────────────
//
// Each of the five filter-drawer sections (When / Where / What / Tags /
// Search options) defaults OPEN when it currently holds an active filter and
// collapsed otherwise, so applied filters stay visible while the drawer stays
// scannable. The `*SectionOpen` methods drive CollapsibleSection.DefaultOpen;
// the `*FilterCount` methods feed the collapsed-state active indicator.
// Pure functions of the props so transactions_filter_test.go can pin them.

// WhenSectionOpen reports whether the date-range ("When") section holds a
// filter (a start or end date) and should default open.
func (p TransactionsProps) WhenSectionOpen() bool {
	return p.WhenFilterCount() > 0
}

// WhenFilterCount counts the active date bounds (0–2).
func (p TransactionsProps) WhenFilterCount() int {
	n := 0
	if p.FilterStartDate != "" {
		n++
	}
	if p.FilterEndDate != "" {
		n++
	}
	return n
}

// WhereSectionOpen reports whether the location ("Where") section holds a
// connection / account / member filter and should default open.
func (p TransactionsProps) WhereSectionOpen() bool {
	return p.WhereFilterCount() > 0
}

// WhereFilterCount counts the active location filters (0–3).
func (p TransactionsProps) WhereFilterCount() int {
	n := 0
	if p.FilterConnID != "" {
		n++
	}
	if p.FilterAccountID != "" {
		n++
	}
	if p.FilterUserID != "" {
		n++
	}
	return n
}

// WhatSectionOpen reports whether the attributes ("What") section holds a
// category / amount / pending / flagged filter and should default open.
func (p TransactionsProps) WhatSectionOpen() bool {
	return p.WhatFilterCount() > 0
}

// WhatFilterCount counts the active attribute filters (0–5).
func (p TransactionsProps) WhatFilterCount() int {
	n := 0
	if p.FilterCategory != "" {
		n++
	}
	if p.FilterMinAmount != "" {
		n++
	}
	if p.FilterMaxAmount != "" {
		n++
	}
	if p.FilterPending != "" {
		n++
	}
	if p.FilterFlagged != "" {
		n++
	}
	return n
}

// TagsSectionOpen reports whether the Tags section holds a tag filter (in
// either AND or OR mode) and should default open.
func (p TransactionsProps) TagsSectionOpen() bool {
	return len(p.FilterTags) > 0 || len(p.FilterAnyTag) > 0
}

// SearchSectionOpen reports whether the Search-options section deviates from
// its defaults (all fields / contains) and should default open. The quick
// search text itself lives in the always-visible search bar, so only a
// non-default field or match mode opens this section.
func (p TransactionsProps) SearchSectionOpen() bool {
	return (p.FilterSearchField != "" && p.FilterSearchField != "all") ||
		(p.FilterSearchMode != "" && p.FilterSearchMode != "contains")
}

// IsReviewQueue reports whether the current page view is the review queue —
// transactions filtered to the needs-review tag. The `/reviews` admin alias
// redirects to `/transactions?tags=needs-review`, so the reviews queue is
// just this filtered view of the transactions list. Drives the M3 keyboard
// shortcut scope (`scope: 'reviews'`) so the page registers approve/reject/
// skip actions on top of the standard transactions navigation.
func (p TransactionsProps) IsReviewQueue() bool {
	for _, slug := range p.FilterTags {
		if slug == "needs-review" {
			return true
		}
	}
	return false
}
