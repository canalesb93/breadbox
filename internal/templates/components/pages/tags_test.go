//go:build !headless && !lite

package pages

import (
	"strings"
	"testing"
)

func TestBuildTagGroups(t *testing.T) {
	tags := []TagRow{
		{ID: "1", Slug: "zzz-rare", DisplayName: "Zzz Rare", TransactionCount: 3},
		{ID: "2", Slug: "needs-review", DisplayName: "Needs Review", TransactionCount: 12},
		{ID: "3", Slug: "fresh", DisplayName: "Fresh", TransactionCount: 0},
		{ID: "4", Slug: "reconciled", DisplayName: "Reconciled", TransactionCount: 12},
		{ID: "5", Slug: "alpha-unused", DisplayName: "Alpha Unused", TransactionCount: 0},
	}

	groups := BuildTagGroups(tags)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2 (in-use, unused)", len(groups))
	}

	// In-use bucket leads.
	in := groups[0]
	if in.Key != "in-use" || in.Label != "In use" {
		t.Fatalf("group[0] = %q/%q, want in-use/In use", in.Key, in.Label)
	}
	if in.CountLabel != "3 tags" {
		t.Errorf("in-use CountLabel = %q, want %q", in.CountLabel, "3 tags")
	}
	if len(in.Rows) != 3 {
		t.Fatalf("in-use rows = %d, want 3", len(in.Rows))
	}
	// Ordered by count desc; the two count==12 tags tie-break by display name
	// (Needs Review < Reconciled), then the count==3 tag last.
	wantOrder := []string{"needs-review", "reconciled", "zzz-rare"}
	for i, slug := range wantOrder {
		if in.Rows[i].Slug != slug {
			t.Errorf("in-use row %d = %q, want %q", i, in.Rows[i].Slug, slug)
		}
	}

	// Unused bucket sinks last, sorted alphabetically by display name.
	un := groups[1]
	if un.Key != "unused" || un.Label != "Unused" {
		t.Fatalf("group[1] = %q/%q, want unused/Unused", un.Key, un.Label)
	}
	if len(un.Rows) != 2 {
		t.Fatalf("unused rows = %d, want 2", len(un.Rows))
	}
	if un.Rows[0].Slug != "alpha-unused" || un.Rows[1].Slug != "fresh" {
		t.Errorf("unused order = %q,%q, want alpha-unused,fresh", un.Rows[0].Slug, un.Rows[1].Slug)
	}

	// The group search index folds in every member so a filter keeps the
	// label line visible when any row matches.
	if !strings.Contains(in.Search, "needs-review") || !strings.Contains(in.Search, "zzz-rare") {
		t.Errorf("in-use Search missing a member slug: %q", in.Search)
	}
}

func TestBuildTagGroupsOmitsEmptyBuckets(t *testing.T) {
	// All in use → only the In-use group, no empty Unused bucket.
	onlyUsed := BuildTagGroups([]TagRow{
		{ID: "1", Slug: "a", DisplayName: "A", TransactionCount: 1},
	})
	if len(onlyUsed) != 1 || onlyUsed[0].Key != "in-use" {
		t.Fatalf("all-used: got %d groups (%v), want 1 in-use", len(onlyUsed), groupKeys(onlyUsed))
	}

	// All unused → only the Unused group.
	onlyUnused := BuildTagGroups([]TagRow{
		{ID: "1", Slug: "a", DisplayName: "A", TransactionCount: 0},
	})
	if len(onlyUnused) != 1 || onlyUnused[0].Key != "unused" {
		t.Fatalf("all-unused: got %d groups (%v), want 1 unused", len(onlyUnused), groupKeys(onlyUnused))
	}

	// Empty input → no groups.
	if got := BuildTagGroups(nil); len(got) != 0 {
		t.Fatalf("nil input: got %d groups, want 0", len(got))
	}
}

func TestTagRowHelpers(t *testing.T) {
	if got := tagUsagePhrase(0); got != "Not used yet" {
		t.Errorf("tagUsagePhrase(0) = %q", got)
	}
	if got := tagUsagePhrase(1); got != "1 transaction" {
		t.Errorf("tagUsagePhrase(1) = %q", got)
	}
	if got := tagUsagePhrase(5); got != "5 transactions" {
		t.Errorf("tagUsagePhrase(5) = %q", got)
	}

	// Slug shown only when it differs from the display name.
	if tagShowSlug(TagRow{DisplayName: "needs-review", Slug: "needs-review"}) {
		t.Error("tagShowSlug should be false when name == slug")
	}
	if !tagShowSlug(TagRow{DisplayName: "Needs Review", Slug: "needs-review"}) {
		t.Error("tagShowSlug should be true when name differs from slug")
	}

	// Color binds to the avatar var; icon falls back to "tag".
	c := "#f59e0b"
	if got := tagTileColorStyle(&c); got != "--avatar-color: #f59e0b" {
		t.Errorf("tagTileColorStyle = %q", got)
	}
	if got := tagTileColorStyle(nil); got != "" {
		t.Errorf("tagTileColorStyle(nil) = %q, want empty", got)
	}
	if got := tagTileIcon(TagRow{}); got != "tag" {
		t.Errorf("tagTileIcon(empty) = %q, want tag", got)
	}
	ic := "bookmark"
	if got := tagTileIcon(TagRow{Icon: &ic}); got != "bookmark" {
		t.Errorf("tagTileIcon = %q, want bookmark", got)
	}
}

func groupKeys(gs []TagGroup) []string {
	out := make([]string, len(gs))
	for i, g := range gs {
		out[i] = g.Key
	}
	return out
}
