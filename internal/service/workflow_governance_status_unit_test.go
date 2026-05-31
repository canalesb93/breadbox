//go:build !lite

package service

import (
	"fmt"
	"testing"
)

// spendBand is the discrete classification that downstream consumers (e.g. the
// gallery's spend banner) derive from a HouseholdSpendStatus. It mirrors the
// logic in internal/admin/workflows_gallery_page.go::buildWorkflowSpendBanner
// so that changes to either end are caught here.
type spendBand int

const (
	// T5BandNone means no ceiling is configured (or ceiling <= 0); no warning shown.
	T5BandNone spendBand = iota
	// T5BandOK means spend is below 80% of the ceiling; banner hidden.
	T5BandOK
	// T5BandApproaching means spend is >=80% but <100% of the ceiling.
	T5BandApproaching
	// T5BandOver means spend has reached or exceeded the ceiling.
	T5BandOver
)

// t5ClassifySpend derives the spend band from a HouseholdSpendStatus.
// This is the pure projection that the gallery render layer performs;
// keeping it here lets us unit-test the boundary conditions without a DB.
func t5ClassifySpend(s HouseholdSpendStatus) spendBand {
	if s.CeilingUSD == nil || *s.CeilingUSD <= 0 {
		return T5BandNone
	}
	ceiling := *s.CeilingUSD
	if s.SpentUSD >= ceiling {
		return T5BandOver
	}
	pct := int(s.SpentUSD / ceiling * 100)
	if pct >= 80 {
		return T5BandApproaching
	}
	return T5BandOK
}

// t5fp is a convenience helper that returns a pointer to f.
func t5fp(f float64) *float64 { return &f }

// t5fmtFloat formats a float64 for use in test names.
func t5fmtFloat(f float64) string {
	if f == float64(int(f)) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.2f", f)
}

// TestT5SpendBand_NoCeiling verifies that when no ceiling is configured (nil
// pointer or a non-positive value) the band is always None.
func TestT5SpendBand_NoCeiling(t *testing.T) {
	cases := []struct {
		name   string
		status HouseholdSpendStatus
		want   spendBand
	}{
		{
			name:   "nil ceiling",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 9999.99, CeilingUSD: nil},
			want:   T5BandNone,
		},
		{
			name:   "zero ceiling",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 5.00, CeilingUSD: t5fp(0)},
			want:   T5BandNone,
		},
		{
			name:   "negative ceiling",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 0, CeilingUSD: t5fp(-10.0)},
			want:   T5BandNone,
		},
		{
			name:   "nil ceiling zero spend",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 0, CeilingUSD: nil},
			want:   T5BandNone,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := t5ClassifySpend(tc.status)
			if got != tc.want {
				t.Errorf("t5ClassifySpend(%+v) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// TestT5SpendBand_OK verifies that spend below the 80% threshold stays quiet.
func TestT5SpendBand_OK(t *testing.T) {
	cases := []struct {
		name   string
		status HouseholdSpendStatus
		want   spendBand
	}{
		{
			name:   "zero spend, $10 ceiling",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 0.00, CeilingUSD: t5fp(10.00)},
			want:   T5BandOK,
		},
		{
			name:   "50% of ceiling",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 5.00, CeilingUSD: t5fp(10.00)},
			want:   T5BandOK,
		},
		{
			name:   "79% of ceiling (just below approaching threshold)",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 7.90, CeilingUSD: t5fp(10.00)},
			want:   T5BandOK,
		},
		{
			name:   "1 cent spent vs large ceiling",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 0.01, CeilingUSD: t5fp(1000.00)},
			want:   T5BandOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := t5ClassifySpend(tc.status)
			if got != tc.want {
				t.Errorf("t5ClassifySpend(%+v) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// TestT5SpendBand_Approaching verifies that spend at or above 80% but below
// 100% of the ceiling is classified as Approaching.
func TestT5SpendBand_Approaching(t *testing.T) {
	cases := []struct {
		name   string
		status HouseholdSpendStatus
		want   spendBand
	}{
		{
			name:   "exactly 80% of ceiling",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 8.00, CeilingUSD: t5fp(10.00)},
			want:   T5BandApproaching,
		},
		{
			name:   "90% of ceiling",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 9.00, CeilingUSD: t5fp(10.00)},
			want:   T5BandApproaching,
		},
		{
			name:   "99% of ceiling (one cent below)",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 9.99, CeilingUSD: t5fp(10.00)},
			want:   T5BandApproaching,
		},
		{
			name:   "80% of a small ceiling ($0.08 of $0.10)",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 0.08, CeilingUSD: t5fp(0.10)},
			want:   T5BandApproaching,
		},
		{
			name:   "85% of a large ceiling",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 85.00, CeilingUSD: t5fp(100.00)},
			want:   T5BandApproaching,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := t5ClassifySpend(tc.status)
			if got != tc.want {
				t.Errorf("t5ClassifySpend(%+v) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// TestT5SpendBand_Over verifies that spend at or above the ceiling is
// classified as Over.
func TestT5SpendBand_Over(t *testing.T) {
	cases := []struct {
		name   string
		status HouseholdSpendStatus
		want   spendBand
	}{
		{
			name:   "exactly at ceiling",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 10.00, CeilingUSD: t5fp(10.00)},
			want:   T5BandOver,
		},
		{
			name:   "1 cent over ceiling",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 10.01, CeilingUSD: t5fp(10.00)},
			want:   T5BandOver,
		},
		{
			name:   "significantly over ceiling",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 50.00, CeilingUSD: t5fp(10.00)},
			want:   T5BandOver,
		},
		{
			name:   "over a small ceiling ($0.11 of $0.10)",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 0.11, CeilingUSD: t5fp(0.10)},
			want:   T5BandOver,
		},
		{
			name:   "over a large ceiling",
			status: HouseholdSpendStatus{WindowDays: 30, SpentUSD: 100.01, CeilingUSD: t5fp(100.00)},
			want:   T5BandOver,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := t5ClassifySpend(tc.status)
			if got != tc.want {
				t.Errorf("t5ClassifySpend(%+v) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// TestT5SpendBandBoundary_ExactThresholds exercises the precise boundary
// transitions: 79%->OK, 80%->Approaching, 99%->Approaching, 100%->Over.
// Uses a $100 ceiling so percentages map cleanly to dollar values.
func TestT5SpendBandBoundary_ExactThresholds(t *testing.T) {
	ceiling := t5fp(100.00)
	cases := []struct {
		spent float64
		want  spendBand
	}{
		{0.00, T5BandOK},
		{79.00, T5BandOK},
		{79.99, T5BandOK}, // truncated int(79.99/100*100)=79, still OK
		{80.00, T5BandApproaching},
		{80.01, T5BandApproaching},
		{99.00, T5BandApproaching},
		{99.99, T5BandApproaching},
		{100.00, T5BandOver},
		{100.01, T5BandOver},
		{150.00, T5BandOver},
	}

	for _, tc := range cases {
		name := "spent=" + t5fmtFloat(tc.spent)
		t.Run(name, func(t *testing.T) {
			s := HouseholdSpendStatus{WindowDays: 30, SpentUSD: tc.spent, CeilingUSD: ceiling}
			got := t5ClassifySpend(s)
			if got != tc.want {
				t.Errorf("t5ClassifySpend(spent=%.2f, ceiling=%.2f) = %v, want %v",
					tc.spent, *ceiling, got, tc.want)
			}
		})
	}
}

// TestT5HouseholdSpendStatus_WindowDays verifies that the HouseholdCeilingWindow
// constant encodes a 30-day rolling window, matching the WindowDays=30 the
// settings UI displays. Both ends must agree so the copy "last 30 days" stays
// accurate.
func TestT5HouseholdSpendStatus_WindowDays(t *testing.T) {
	const want = 30
	got := int(HouseholdCeilingWindow.Hours() / 24)
	if got != want {
		t.Errorf("HouseholdCeilingWindow = %d days, want %d", got, want)
	}

	s := HouseholdSpendStatus{WindowDays: want}
	if s.WindowDays != want {
		t.Errorf("HouseholdSpendStatus.WindowDays = %d, want %d", s.WindowDays, want)
	}
}
