package service

import "testing"

func TestFormatCurrency(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want string
	}{
		{"zero", 0, "$0.00"},
		{"cents only", 0.05, "$0.05"},
		{"one cent rounding", 0.004, "$0.00"},
		{"half-cent rounds up", 0.005, "$0.01"},
		{"under thousand", 123.45, "$123.45"},
		{"exactly thousand", 1000, "$1,000.00"},
		{"thousands separator", 12345.67, "$12,345.67"},
		{"millions separator", 1234567.89, "$1,234,567.89"},
		{"billion", 1000000000, "$1,000,000,000.00"},
		{"truncates to two decimals", 9.999, "$10.00"},
		{"carry rolls into thousands", 999.999, "$1,000.00"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatCurrency(tc.in)
			if got != tc.want {
				t.Errorf("FormatCurrency(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
