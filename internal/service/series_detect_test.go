//go:build !lite

package service

import (
	"testing"
	"time"
)

func cp(date string, cents int64) chargePoint {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		panic(err)
	}
	return chargePoint{date: t, amountCents: cents}
}

func TestAnalyzeGroup_CleanMonthlyTight(t *testing.T) {
	g, ok := analyzeGroup([]chargePoint{
		cp("2026-01-15", 999), cp("2026-02-15", 999), cp("2026-03-15", 999),
	}, "spotify", "USD")
	if !ok {
		t.Fatal("expected clean monthly to qualify")
	}
	if g.cadence != SeriesCadenceMonthly {
		t.Errorf("cadence = %q, want monthly", g.cadence)
	}
	if g.signals.AmountBranch != "tight" {
		t.Errorf("branch = %q, want tight", g.signals.AmountBranch)
	}
	if g.expectedAmountCents != 999 {
		t.Errorf("expected_amount = %d, want 999", g.expectedAmountCents)
	}
	if g.expectedDay == nil || *g.expectedDay != 15 {
		t.Errorf("expected_day = %v, want 15", g.expectedDay)
	}
}

func TestAnalyzeGroup_AnnualTwoCharges(t *testing.T) {
	g, ok := analyzeGroup([]chargePoint{
		cp("2025-04-01", 9900), cp("2026-04-01", 9900),
	}, "aws", "USD")
	if !ok {
		t.Fatal("expected 2-charge annual to qualify (floor 2)")
	}
	if g.cadence != SeriesCadenceAnnual {
		t.Errorf("cadence = %q, want annual", g.cadence)
	}
}

func TestAnalyzeGroup_MonotonicDriftAccepted(t *testing.T) {
	// A spread wider than the $1 tight band but monotonic with bounded steps —
	// a real multi-year price-hike subscription. $10.00 → $11.50 → $13.00.
	g, ok := analyzeGroup([]chargePoint{
		cp("2026-01-15", 1000), cp("2026-02-15", 1150), cp("2026-03-15", 1300),
	}, "netflix", "USD")
	if !ok {
		t.Fatal("expected monthly price-hike series to qualify")
	}
	if g.signals.AmountBranch != "monotonic_drift" {
		t.Errorf("branch = %q, want monotonic_drift", g.signals.AmountBranch)
	}
	if g.expectedAmountCents != 1300 {
		t.Errorf("expected_amount = %d, want 1300 (latest/current price)", g.expectedAmountCents)
	}
}

func TestAnalyzeGroup_Rejections(t *testing.T) {
	cases := []struct {
		name    string
		charges []chargePoint
	}{
		{"irregular gaps", []chargePoint{cp("2026-01-01", 500), cp("2026-01-06", 500), cp("2026-04-06", 500)}},
		{"amount scatter", []chargePoint{cp("2026-01-15", 999), cp("2026-02-15", 4999), cp("2026-03-15", 999)}},
		{"too few for monthly", []chargePoint{cp("2026-01-15", 999), cp("2026-02-15", 999)}},
		{"same-day duplicates", []chargePoint{cp("2026-01-15", 999), cp("2026-01-15", 999), cp("2026-01-15", 999)}},
		{"single charge", []chargePoint{cp("2026-01-15", 999)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, ok := analyzeGroup(c.charges, "x", "USD"); ok {
				t.Errorf("expected %q to be rejected", c.name)
			}
		})
	}
}

func TestSnapCadence(t *testing.T) {
	cases := []struct {
		gap  float64
		want string
	}{
		{7, SeriesCadenceWeekly},
		{14, SeriesCadenceBiweekly},
		{30, SeriesCadenceMonthly},
		{31, SeriesCadenceMonthly},
		{91, SeriesCadenceQuarterly},
		{365, SeriesCadenceAnnual},
		{47, SeriesCadenceIrregular}, // between monthly and quarterly, snaps to neither
	}
	for _, c := range cases {
		if got, _ := snapCadence(c.gap); got != c.want {
			t.Errorf("snapCadence(%v) = %q, want %q", c.gap, got, c.want)
		}
	}
}
