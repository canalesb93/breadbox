//go:build !headless && !lite

package components

import "testing"

func TestAmountText(t *testing.T) {
	tests := []struct {
		name  string
		props AmountProps
		want  string
	}{
		// Default intent: AmountTransaction
		{"transaction expense", AmountProps{Value: 12.34}, "$12.34"},
		{"transaction income (neg)", AmountProps{Value: -12.34}, "+$12.34"},
		{"transaction zero", AmountProps{Value: 0}, "$0.00"},
		{"transaction big expense", AmountProps{Value: 1234567.89}, "$1,234,567.89"},
		{"transaction big income", AmountProps{Value: -1234.56}, "+$1,234.56"},

		// Balance intent: signed, no sign manipulation
		{"balance positive", AmountProps{Value: 1234.56, Intent: AmountBalance}, "$1,234.56"},
		{"balance negative", AmountProps{Value: -1234.56, Intent: AmountBalance}, "-$1,234.56"},
		{"balance zero", AmountProps{Value: 0, Intent: AmountBalance}, "$0.00"},

		// Cost intent: always absolute
		{"cost positive", AmountProps{Value: 0.42, Intent: AmountCost}, "$0.42"},
		{"cost negative absorbed", AmountProps{Value: -0.42, Intent: AmountCost}, "$0.42"},
		{"cost zero", AmountProps{Value: 0, Intent: AmountCost}, "$0.00"},
		{"cost precision 4", AmountProps{Value: 0.1234, Intent: AmountCost, Precision: 4}, "$0.1234"},
		{"cost precision 4 over a buck", AmountProps{Value: 1.5, Intent: AmountCost, Precision: 4}, "$1.5000"},

		// Format: abbreviated (≥ $1M)
		{"abbreviated under 1M", AmountProps{Value: 999_999.99, Intent: AmountBalance, Format: AmountFormatAbbreviated}, "$999,999.99"},
		{"abbreviated exactly 1M", AmountProps{Value: 1_000_000, Intent: AmountBalance, Format: AmountFormatAbbreviated}, "$1.0M"},
		{"abbreviated 1.2M", AmountProps{Value: 1_234_567, Intent: AmountBalance, Format: AmountFormatAbbreviated}, "$1.2M"},
		{"abbreviated negative balance", AmountProps{Value: -1_500_000, Intent: AmountBalance, Format: AmountFormatAbbreviated}, "-$1.5M"},

		// Format: compact
		{"compact whole", AmountProps{Value: 50, Intent: AmountCost, Format: AmountFormatCompact}, "$50"},
		{"compact decimal", AmountProps{Value: 12.34, Intent: AmountCost, Format: AmountFormatCompact}, "$12.34"},
		{"compact whole large", AmountProps{Value: 12_345, Intent: AmountCost, Format: AmountFormatCompact}, "$12,345"},

		// Rounding edge: 99.999 → $100.00
		{"rounds cents over", AmountProps{Value: 99.999, Intent: AmountBalance}, "$100.00"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := AmountText(tc.props); got != tc.want {
				t.Errorf("AmountText(%+v) = %q, want %q", tc.props, got, tc.want)
			}
		})
	}
}

func TestAmountClasses(t *testing.T) {
	tests := []struct {
		name  string
		props AmountProps
		want  string
	}{
		{"expense default", AmountProps{Value: 12.34}, "tabular-nums"},
		{"income default", AmountProps{Value: -12.34}, "tabular-nums text-success"},
		{"income pending", AmountProps{Value: -12.34, Pending: true}, "tabular-nums text-success bb-tx-amount--pending"},
		{"balance negative no success color", AmountProps{Value: -100, Intent: AmountBalance}, "tabular-nums"},
		{"cost no color", AmountProps{Value: -100, Intent: AmountCost}, "tabular-nums"},
		{"with extra class", AmountProps{Value: 1, Class: "text-3xl font-semibold"}, "tabular-nums text-3xl font-semibold"},
		{"income with extra class", AmountProps{Value: -1, Class: "text-3xl"}, "tabular-nums text-success text-3xl"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := amountClasses(tc.props); got != tc.want {
				t.Errorf("amountClasses(%+v) = %q, want %q", tc.props, got, tc.want)
			}
		})
	}
}
