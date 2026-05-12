package client

import (
	"testing"
)

// TestTransactionFiltersQuery checks the filter → query-string mapping.
// Future filter shape changes should preserve these param names: the CLI
// and any agent's query-builder both depend on them.
func TestTransactionFiltersQuery(t *testing.T) {
	pending := false
	min := 5.5
	max := 100.0

	tests := []struct {
		name string
		f    TransactionFilters
		want map[string]string
	}{
		{
			name: "empty filters produce no params",
			f:    TransactionFilters{},
			want: map[string]string{},
		},
		{
			name: "primary filters round-trip",
			f: TransactionFilters{
				Account:       "acc1",
				Category:      "groceries",
				From:          "2025-01-01",
				To:            "2025-02-01",
				Search:        "starbucks",
				SearchMode:    "fuzzy",
				ExcludeSearch: "refund",
				User:          "u1",
			},
			want: map[string]string{
				"account_id":     "acc1",
				"category_slug":  "groceries",
				"start_date":     "2025-01-01",
				"end_date":       "2025-02-01",
				"search":         "starbucks",
				"search_mode":    "fuzzy",
				"exclude_search": "refund",
				"user_id":        "u1",
			},
		},
		{
			name: "tags join with commas",
			f:    TransactionFilters{Tags: []string{"foo", "bar"}, AnyTags: []string{"baz"}},
			want: map[string]string{
				"tags":    "foo,bar",
				"any_tag": "baz",
			},
		},
		{
			name: "amount + pending pointers serialise",
			f:    TransactionFilters{MinAmount: &min, MaxAmount: &max, Pending: &pending},
			want: map[string]string{
				"min_amount": "5.5",
				"max_amount": "100",
				"pending":    "false",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.f.Query()
			if len(got) != len(tc.want) {
				t.Fatalf("got %d params, want %d: %v vs %v", len(got), len(tc.want), got, tc.want)
			}
			for k, v := range tc.want {
				if got.Get(k) != v {
					t.Errorf("%s: got %q, want %q", k, got.Get(k), v)
				}
			}
		})
	}
}

// TestJoinCSV asserts the helper produces stable comma-joined values
// without trailing commas or quoting.
func TestJoinCSV(t *testing.T) {
	if joinCSV([]string{}) != "" {
		t.Fatalf("empty slice should return empty string")
	}
	if joinCSV([]string{"a"}) != "a" {
		t.Fatalf("single element should not get a trailing comma")
	}
	if joinCSV([]string{"a", "b", "c"}) != "a,b,c" {
		t.Fatalf("multi-element output unexpected")
	}
}
