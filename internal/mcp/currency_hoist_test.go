//go:build !lite

package mcp

import "testing"

func strptr(s string) *string { return &s }

func TestHoistUniformCurrency(t *testing.T) {
	tests := []struct {
		name      string
		rows      []map[string]any
		wantCur   string
		wantOK    bool
		wantStrip bool // per-row iso_currency_code removed on success
	}{
		{
			name:      "all USD via *string hoists and strips",
			rows:      []map[string]any{{"iso_currency_code": strptr("USD")}, {"iso_currency_code": strptr("USD")}},
			wantCur:   "USD",
			wantOK:    true,
			wantStrip: true,
		},
		{
			name:    "all USD via plain string hoists",
			rows:    []map[string]any{{"iso_currency_code": "USD"}, {"iso_currency_code": "USD"}},
			wantCur: "USD",
			wantOK:  true,
		},
		{
			name:   "mixed currencies stays per-row",
			rows:   []map[string]any{{"iso_currency_code": strptr("USD")}, {"iso_currency_code": strptr("EUR")}},
			wantOK: false,
		},
		{
			name:   "a nil currency disables hoisting",
			rows:   []map[string]any{{"iso_currency_code": strptr("USD")}, {"iso_currency_code": (*string)(nil)}},
			wantOK: false,
		},
		{
			name:   "an empty-string currency disables hoisting",
			rows:   []map[string]any{{"iso_currency_code": "USD"}, {"iso_currency_code": ""}},
			wantOK: false,
		},
		{
			name:   "field absent (not in projection) disables hoisting",
			rows:   []map[string]any{{"amount": 1.0}, {"amount": 2.0}},
			wantOK: false,
		},
		{
			name:   "empty list disables hoisting",
			rows:   []map[string]any{},
			wantOK: false,
		},
		{
			name:      "single row hoists",
			rows:      []map[string]any{{"iso_currency_code": strptr("GBP")}},
			wantCur:   "GBP",
			wantOK:    true,
			wantStrip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cur, ok := hoistUniformCurrency(tt.rows)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && cur != tt.wantCur {
				t.Errorf("cur = %q, want %q", cur, tt.wantCur)
			}
			if tt.wantStrip {
				for i, r := range tt.rows {
					if _, present := r["iso_currency_code"]; present {
						t.Errorf("row %d still carries iso_currency_code after successful hoist", i)
					}
				}
			}
			if !tt.wantOK {
				// Non-hoist outcomes must leave rows untouched.
				for i, r := range tt.rows {
					if _, hadCur := r["iso_currency_code"]; !hadCur {
						continue
					}
					if _, present := r["iso_currency_code"]; !present {
						t.Errorf("row %d lost iso_currency_code despite no hoist", i)
					}
				}
			}
		})
	}
}

func TestCurrencyString(t *testing.T) {
	tests := []struct {
		name    string
		in      any
		wantStr string
		wantOK  bool
	}{
		{"plain string", "USD", "USD", true},
		{"empty string", "", "", false},
		{"non-nil pointer", strptr("EUR"), "EUR", true},
		{"nil pointer", (*string)(nil), "", false},
		{"empty pointer", strptr(""), "", false},
		{"wrong type", 42, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, ok := currencyString(tt.in)
			if ok != tt.wantOK || s != tt.wantStr {
				t.Errorf("currencyString(%v) = (%q, %v), want (%q, %v)", tt.in, s, ok, tt.wantStr, tt.wantOK)
			}
		})
	}
}
