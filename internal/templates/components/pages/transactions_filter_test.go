//go:build !headless && !lite

package pages

import "testing"

// TestFilterSectionSmartDefaults pins the open-by-default + active-count logic
// that drives the /transactions filter drawer's CollapsibleSection headers:
// a section defaults open exactly when it holds an active filter, and the
// collapsed-state count reflects how many of that section's filters are set.
func TestFilterSectionSmartDefaults(t *testing.T) {
	t.Run("empty props — every section collapsed, zero counts", func(t *testing.T) {
		var p TransactionsProps
		if p.WhenSectionOpen() || p.WhereSectionOpen() || p.WhatSectionOpen() ||
			p.TagsSectionOpen() || p.SearchSectionOpen() {
			t.Fatalf("no filters set, but a section defaulted open: when=%v where=%v what=%v tags=%v search=%v",
				p.WhenSectionOpen(), p.WhereSectionOpen(), p.WhatSectionOpen(),
				p.TagsSectionOpen(), p.SearchSectionOpen())
		}
		if got := p.WhenFilterCount() + p.WhereFilterCount() + p.WhatFilterCount(); got != 0 {
			t.Fatalf("expected zero total active-filter count, got %d", got)
		}
	})

	t.Run("When opens on either date bound", func(t *testing.T) {
		start := TransactionsProps{FilterStartDate: "2026-01-01"}
		if !start.WhenSectionOpen() || start.WhenFilterCount() != 1 {
			t.Errorf("start-only: open=%v count=%d, want open=true count=1", start.WhenSectionOpen(), start.WhenFilterCount())
		}
		both := TransactionsProps{FilterStartDate: "2026-01-01", FilterEndDate: "2026-02-01"}
		if both.WhenFilterCount() != 2 {
			t.Errorf("both bounds: count=%d, want 2", both.WhenFilterCount())
		}
	})

	t.Run("Where counts connection/account/member", func(t *testing.T) {
		p := TransactionsProps{FilterConnID: "c", FilterAccountID: "a", FilterUserID: "u"}
		if !p.WhereSectionOpen() || p.WhereFilterCount() != 3 {
			t.Errorf("open=%v count=%d, want open=true count=3", p.WhereSectionOpen(), p.WhereFilterCount())
		}
	})

	t.Run("What counts category/amounts/pending/flagged", func(t *testing.T) {
		p := TransactionsProps{
			FilterCategory:  "groceries",
			FilterMinAmount: "5",
			FilterMaxAmount: "50",
			FilterPending:   "true",
			FilterFlagged:   "false",
		}
		if !p.WhatSectionOpen() || p.WhatFilterCount() != 5 {
			t.Errorf("open=%v count=%d, want open=true count=5", p.WhatSectionOpen(), p.WhatFilterCount())
		}
	})

	t.Run("Tags opens in AND or OR mode", func(t *testing.T) {
		and := TransactionsProps{FilterTags: []string{"x"}}
		or := TransactionsProps{FilterAnyTag: []string{"y", "z"}}
		if !and.TagsSectionOpen() || !or.TagsSectionOpen() {
			t.Errorf("tags should open: and=%v or=%v", and.TagsSectionOpen(), or.TagsSectionOpen())
		}
	})

	t.Run("Search opens only on non-default field or mode", func(t *testing.T) {
		// Defaults (all / contains) stay collapsed.
		defaults := []TransactionsProps{
			{},
			{FilterSearchField: "all"},
			{FilterSearchMode: "contains"},
			{FilterSearchField: "all", FilterSearchMode: "contains"},
		}
		for i, p := range defaults {
			if p.SearchSectionOpen() {
				t.Errorf("case %d: defaults should stay collapsed, got open", i)
			}
		}
		nonDefault := []TransactionsProps{
			{FilterSearchField: "merchant"},
			{FilterSearchMode: "fuzzy"},
		}
		for i, p := range nonDefault {
			if !p.SearchSectionOpen() {
				t.Errorf("case %d: non-default search option should open the section", i)
			}
		}
	})
}

// TestTxDatePresets verifies the quick date presets are well-formed: four
// presets, each with a non-empty label and ISO YYYY-MM-DD bounds where start
// is not after end.
func TestTxDatePresets(t *testing.T) {
	presets := txDatePresets()
	if len(presets) != 4 {
		t.Fatalf("expected 4 date presets, got %d", len(presets))
	}
	for _, p := range presets {
		if p.Label == "" {
			t.Errorf("preset has empty label: %+v", p)
		}
		if len(p.Start) != 10 || len(p.End) != 10 {
			t.Errorf("preset %q has non-ISO bounds: start=%q end=%q", p.Label, p.Start, p.End)
		}
		if p.Start > p.End {
			t.Errorf("preset %q start %q is after end %q", p.Label, p.Start, p.End)
		}
	}
}
