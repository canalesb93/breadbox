package pages

import (
	"breadbox/internal/service"
	"breadbox/internal/templates/components"
)

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
