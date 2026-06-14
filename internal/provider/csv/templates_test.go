//go:build !lite

package csv

import "testing"

// TestDetectTemplate exercises bank-format detection: exact (case-insensitive)
// header matching, the "extra trailing columns are allowed" rule, the
// disambiguation between templates that share a leading column, and the
// no-match / too-few-columns fallbacks. These functions drive CSV import
// column mapping, so a silent regression here mismaps users' transactions.
func TestDetectTemplate(t *testing.T) {
	cases := []struct {
		name    string
		headers []string
		want    string // template Name, or "" for nil
	}{
		{
			name:    "chase credit card exact",
			headers: []string{"Transaction Date", "Post Date", "Description", "Category", "Type", "Amount", "Memo"},
			want:    "Chase Credit Card",
		},
		{
			// Shares the leading "Transaction Date" column with Chase Credit
			// Card; the second column ("Posted Date" vs "Post Date") must
			// disambiguate to Capital One.
			name:    "capital one not confused with chase",
			headers: []string{"Transaction Date", "Posted Date", "Card No.", "Description", "Category", "Debit", "Credit"},
			want:    "Capital One",
		},
		{
			name:    "case insensitive match",
			headers: []string{"date", "description", "amount", "running bal."},
			want:    "Bank of America",
		},
		{
			// Extra trailing columns beyond the pattern are permitted.
			name:    "extra trailing columns allowed",
			headers: []string{"Date", "Description", "Amount", "Running Bal.", "Extra Column"},
			want:    "Bank of America",
		},
		{
			// Fewer columns than the shortest matching pattern → no match.
			name:    "too few columns",
			headers: []string{"Date", "Description"},
			want:    "",
		},
		{
			name:    "unknown headers",
			headers: []string{"Foo", "Bar", "Baz"},
			want:    "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DetectTemplate(c.headers)
			if c.want == "" {
				if got != nil {
					t.Fatalf("DetectTemplate(%v) = %q, want nil", c.headers, got.Name)
				}
				return
			}
			if got == nil {
				t.Fatalf("DetectTemplate(%v) = nil, want %q", c.headers, c.want)
			}
			if got.Name != c.want {
				t.Errorf("DetectTemplate(%v) = %q, want %q", c.headers, got.Name, c.want)
			}
		})
	}
}

// TestDetectColumns covers the generic header-pattern fallback used when no
// bank template matches: field detection, pattern priority (an earlier
// synonym wins over a later one), the "payee" vs "payee name" split across
// description/merchant_name, and the empty result for unrecognized headers.
func TestDetectColumns(t *testing.T) {
	t.Run("basic fields", func(t *testing.T) {
		got := DetectColumns([]string{"Date", "Description", "Amount"})
		want := map[string]int{"date": 0, "description": 1, "amount": 2}
		assertColumns(t, got, want)
	})

	t.Run("synonyms map to canonical fields", func(t *testing.T) {
		// "Posting Date" → date, "Memo" → description, "Value" → amount,
		// "Type" → category.
		got := DetectColumns([]string{"Posting Date", "Memo", "Value", "Type"})
		want := map[string]int{"date": 0, "description": 1, "amount": 2, "category": 3}
		assertColumns(t, got, want)
	})

	t.Run("higher-priority synonym wins", func(t *testing.T) {
		// Both "Description" and "Details" are description synonyms; the
		// earlier pattern ("description") must win over "details".
		got := DetectColumns([]string{"Description", "Details", "Amount"})
		if got["description"] != 0 {
			t.Errorf("description = %d, want 0 (Description should beat Details)", got["description"])
		}
	})

	t.Run("payee vs payee name split", func(t *testing.T) {
		// "payee" is a description synonym; "payee name" is a merchant_name
		// synonym — they must not collapse onto the same field.
		got := DetectColumns([]string{"Date", "Payee Name", "Amount"})
		if _, ok := got["merchant_name"]; !ok || got["merchant_name"] != 1 {
			t.Errorf("merchant_name = %v, want 1", got["merchant_name"])
		}
		if _, ok := got["description"]; ok {
			t.Errorf("description should be unset for a 'Payee Name' header, got %d", got["description"])
		}
	})

	t.Run("no recognized headers", func(t *testing.T) {
		got := DetectColumns([]string{"Foo", "Bar"})
		if len(got) != 0 {
			t.Errorf("DetectColumns(unknown) = %v, want empty", got)
		}
	})
}

func assertColumns(t *testing.T, got, want map[string]int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("field %q = %d, want %d (full: %v)", k, got[k], v, got)
		}
	}
}
