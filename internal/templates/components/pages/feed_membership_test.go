//go:build !headless && !lite

package pages

import "testing"

// TestFeedBulkMembershipVerbs locks the headline verbs for series /
// counterparty membership bulk-action buckets so they read legibly instead
// of falling through to the generic "updated".
func TestFeedBulkMembershipVerbs(t *testing.T) {
	cases := map[string]string{
		"series_assigned":       "assigned to a series",
		"series_unlinked":       "removed from a series",
		"counterparty_assigned": "assigned to a counterparty",
		"counterparty_unlinked": "removed from a counterparty",
		// regression guard: the pre-existing kinds keep their verbs.
		"category_set": "categorised",
		"rule_applied": "ran a rule on",
		// unknown kinds still fall through to the generic verb.
		"something_new": "updated",
	}
	for kind, want := range cases {
		if got := feedBulkVerb(kind); got != want {
			t.Errorf("feedBulkVerb(%q) = %q, want %q", kind, got, want)
		}
	}
}

// TestFeedBulkMembershipSubjectLabel locks the single-subject phrasing for
// membership buckets ("to the Netflix series", "to Amazon").
func TestFeedBulkMembershipSubjectLabel(t *testing.T) {
	cases := []struct {
		kind string
		name string
		want string
	}{
		{"series_assigned", "Netflix", "to the Netflix series"},
		{"series_unlinked", "Netflix", "from the Netflix series"},
		{"counterparty_assigned", "Amazon", "to Amazon"},
		{"counterparty_unlinked", "Amazon", "from Amazon"},
		{"category_set", "Groceries", "as Groceries"},
	}
	for _, c := range cases {
		b := &FeedBulkAction{
			Kind:     c.kind,
			Subjects: []FeedBulkSubject{{Name: c.name, Count: 3}},
		}
		if got := feedBulkSubjectLabel(b); got != c.want {
			t.Errorf("feedBulkSubjectLabel(kind=%q name=%q) = %q, want %q", c.kind, c.name, got, c.want)
		}
	}
}

// TestFeedBulkKindBreakdownMembership verifies mixed-kind breakdown lines name
// membership kinds with friendly labels instead of the raw kind string.
func TestFeedBulkKindBreakdownMembership(t *testing.T) {
	b := &FeedBulkAction{
		Kind: "mixed",
		KindCounts: map[string]int{
			"series_assigned":       5,
			"counterparty_assigned": 2,
			"category_set":          3,
		},
	}
	got := feedBulkKindBreakdown(b)
	// Order is fixed: category_set first, then series, then counterparty.
	want := "3 categorised · 5 series-assigned · 2 counterparty-assigned"
	if got != want {
		t.Errorf("feedBulkKindBreakdown = %q, want %q", got, want)
	}
	// No raw kind string should leak into the rendered breakdown.
	for _, raw := range []string{"series_assigned", "counterparty_assigned"} {
		if contains(got, raw) {
			t.Errorf("breakdown %q leaked raw kind %q", got, raw)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
