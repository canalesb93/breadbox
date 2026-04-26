package admin

import (
	"testing"
)

// titleCaseMerchant coverage moved to internal/templates/components.TestTitleCase
// after PR #513 consolidated the helper. Don't reintroduce a wrapper test here
// — extend TestTitleCase in the components package instead.

// commaInt / fmtBalance funcMap coverage was deleted alongside the funcMap
// entries themselves once html/template consumers migrated to templ; the
// underlying components.CommaInt / components.CommaAmount helpers remain
// covered by TestCommaInt / TestCommaAmount in the components package.

func TestRuleFieldLabel(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"provider_name", "Name"},
		{"provider_merchant_name", "Merchant"},
		{"amount", "Amount"},
		{"pending", "Pending"},
		{"category", "Category"},
		{"provider_category_primary", "Category (primary)"},
		{"provider_category_detailed", "Category (detail)"},
		{"tags", "Tag"},
		{"account_name", "Account"},
		{"user_name", "Family member"},
		{"provider", "Provider"},
		{"", "—"},
		// Unknown fields fall back to title-cased display.
		{"some_custom_field", "Some Custom Field"},
		{"foo", "Foo"},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got := ruleFieldLabel(tc.in)
			if got != tc.want {
				t.Errorf("ruleFieldLabel(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRuleOpLabel(t *testing.T) {
	tests := []struct {
		name  string
		op    string
		field string
		want  string
	}{
		{"contains on string", "contains", "name", "contains"},
		{"contains on tags", "contains", "tags", "has"},
		{"not_contains on string", "not_contains", "name", "does not contain"},
		{"not_contains on tags", "not_contains", "tags", "does not have"},
		{"in on string", "in", "name", "in"},
		{"in on tags", "in", "tags", "has any of"},
		{"matches regex", "matches", "name", "matches /regex/"},
		{"eq on amount (numeric)", "eq", "amount", "="},
		{"eq on pending (bool)", "eq", "pending", "is"},
		{"eq on string", "eq", "name", "is"},
		{"neq on amount (numeric)", "neq", "amount", "≠"},
		{"neq on pending (bool)", "neq", "pending", "is not"},
		{"neq on string", "neq", "name", "is not"},
		{"gt", "gt", "amount", ">"},
		{"gte", "gte", "amount", "≥"},
		{"lt", "lt", "amount", "<"},
		{"lte", "lte", "amount", "≤"},
		{"empty op", "", "name", "—"},
		{"unknown op passes through", "weirdly", "name", "weirdly"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ruleOpLabel(tc.op, tc.field)
			if got != tc.want {
				t.Errorf("ruleOpLabel(%q, %q) = %q, want %q", tc.op, tc.field, got, tc.want)
			}
		})
	}
}

func TestRuleValueFormat(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string", "coffee", "coffee"},
		{"empty string", "", ""},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"int via default", 42, "42"},
		{"float via default", 12.5, "12.5"},
		{"slice of strings", []any{"a", "b", "c"}, "a, b, c"},
		{"slice with mixed types", []any{"tag", 2, true}, "tag, 2, true"},
		{"empty slice", []any{}, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ruleValueFormat(tc.in)
			if got != tc.want {
				t.Errorf("ruleValueFormat(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
