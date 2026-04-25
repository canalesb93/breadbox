package admin

import (
	"testing"
)

// titleCaseMerchant coverage moved to internal/templates/components.TestTitleCase
// after PR #513 consolidated the helper. Don't reintroduce a wrapper test here
// — extend TestTitleCase in the components package instead.

// TestFuncMapCommaInt locks in the commaInt funcMap entry's behavior across the
// supported argument types. The implementation delegates to
// components.CommaInt, but the type-switch wrapper is admin-package-specific.
func TestFuncMapCommaInt(t *testing.T) {
	tr, err := NewTemplateRenderer(nil)
	if err != nil {
		t.Fatalf("NewTemplateRenderer: %v", err)
	}
	fn, ok := tr.funcMap["commaInt"].(func(any) string)
	if !ok {
		t.Fatalf("commaInt funcMap entry missing or wrong signature")
	}
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"int small", int(42), "42"},
		{"int four digits", int(1234), "1,234"},
		{"int64 large", int64(1234567), "1,234,567"},
		{"int zero", int(0), "0"},
		{"unknown type falls back to %v", float64(1.5), "1.5"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := fn(tc.in); got != tc.want {
				t.Errorf("commaInt(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestFuncMapFmtBalance locks in the fmtBalance funcMap entry's behavior. It
// always emits "$X,XXX.XX" with cents and a leading sign for negatives, and
// returns "" for nil pointers or unsupported types.
func TestFuncMapFmtBalance(t *testing.T) {
	tr, err := NewTemplateRenderer(nil)
	if err != nil {
		t.Fatalf("NewTemplateRenderer: %v", err)
	}
	fn, ok := tr.funcMap["fmtBalance"].(func(interface{}) string)
	if !ok {
		t.Fatalf("fmtBalance funcMap entry missing or wrong signature")
	}
	f := func(v float64) *float64 { return &v }
	tests := []struct {
		name string
		in   interface{}
		want string
	}{
		{"float64 zero", float64(0), "$0.00"},
		{"float64 small", float64(12.34), "$12.34"},
		{"float64 thousands", float64(1234.56), "$1,234.56"},
		{"float64 millions", float64(1_234_567.89), "$1,234,567.89"},
		{"float64 negative", float64(-1234.56), "-$1,234.56"},
		{"*float64 value", f(99.5), "$99.50"},
		{"*float64 negative", f(-1000), "-$1,000.00"},
		{"*float64 nil", (*float64)(nil), ""},
		{"unsupported type", "1234", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := fn(tc.in); got != tc.want {
				t.Errorf("fmtBalance(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

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
