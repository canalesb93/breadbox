//go:build !lite

package service

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
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

// --- renewal-health derivation (seriesRenewalHealth) ---

func mkDate(s string) pgtype.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return pgtype.Date{Time: t, Valid: true}
}

func TestSeriesRenewalHealth(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		status       string
		cadence      string
		nextExpected pgtype.Date
		wantHealth   string
		wantDays     *int // nil means "expect nil pointer"
	}{
		{"non-active is empty", SeriesStatusCandidate, SeriesCadenceMonthly, mkDate("2026-06-15"), "", nil},
		{"paused is empty", SeriesStatusPaused, SeriesCadenceMonthly, mkDate("2026-06-15"), "", nil},
		{"no projection → unknown", SeriesStatusActive, SeriesCadenceIrregular, pgtype.Date{}, SeriesHealthUnknown, nil},
		{"future beyond window → active", SeriesStatusActive, SeriesCadenceMonthly, mkDate("2026-06-29"), SeriesHealthActive, intp(30)},
		{"within a week → due_soon", SeriesStatusActive, SeriesCadenceMonthly, mkDate("2026-06-05"), SeriesHealthDueSoon, intp(6)},
		{"today → due_soon", SeriesStatusActive, SeriesCadenceMonthly, mkDate("2026-05-30"), SeriesHealthDueSoon, intp(0)},
		{"a few days late, within cycle → overdue", SeriesStatusActive, SeriesCadenceMonthly, mkDate("2026-05-20"), SeriesHealthOverdue, intp(-10)},
		{"missed a full monthly cycle → stale", SeriesStatusActive, SeriesCadenceMonthly, mkDate("2026-04-01"), SeriesHealthStale, intp(-59)},
		{"weekly 10 days late → stale", SeriesStatusActive, SeriesCadenceWeekly, mkDate("2026-05-20"), SeriesHealthStale, intp(-10)},
		{"weekly 5 days late → overdue", SeriesStatusActive, SeriesCadenceWeekly, mkDate("2026-05-25"), SeriesHealthOverdue, intp(-5)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotHealth, gotDays := seriesRenewalHealth(tc.status, tc.cadence, tc.nextExpected, now)
			if gotHealth != tc.wantHealth {
				t.Errorf("health = %q, want %q", gotHealth, tc.wantHealth)
			}
			switch {
			case tc.wantDays == nil && gotDays != nil:
				t.Errorf("days = %d, want nil", *gotDays)
			case tc.wantDays != nil && gotDays == nil:
				t.Errorf("days = nil, want %d", *tc.wantDays)
			case tc.wantDays != nil && gotDays != nil && *gotDays != *tc.wantDays:
				t.Errorf("days = %d, want %d", *gotDays, *tc.wantDays)
			}
		})
	}
}

func intp(i int) *int { return &i }

// --- evaluateGroup rejection reasons (the explain-feed verdicts) ---

func TestEvaluateGroup_Reasons(t *testing.T) {
	cases := []struct {
		name       string
		charges    []chargePoint
		wantReason string
	}{
		{
			name:       "single charge",
			charges:    []chargePoint{cp("2026-01-15", 999)},
			wantReason: seriesRejectTooFewCharges,
		},
		{
			name:       "all same day",
			charges:    []chargePoint{cp("2026-01-15", 999), cp("2026-01-15", 999), cp("2026-01-15", 999)},
			wantReason: seriesRejectSameDayDuplicates,
		},
		{
			name:       "irregular gaps dont snap",
			charges:    []chargePoint{cp("2026-01-01", 999), cp("2026-02-15", 999), cp("2026-04-01", 999)},
			wantReason: seriesRejectIrregularCadence,
		},
		{
			name:       "only two monthly charges",
			charges:    []chargePoint{cp("2026-01-15", 999), cp("2026-02-15", 999)},
			wantReason: seriesRejectTooFewOccurrences,
		},
		{
			name:       "monthly-ish but intervals too variable",
			charges:    []chargePoint{cp("2026-01-01", 999), cp("2026-01-26", 999), cp("2026-02-25", 999), cp("2026-04-05", 999)},
			wantReason: seriesRejectIntervalVariable,
		},
		{
			name:       "clean monthly cadence but scattered amounts",
			charges:    []chargePoint{cp("2026-01-15", 999), cp("2026-02-15", 2999), cp("2026-03-15", 500), cp("2026-04-15", 1500)},
			wantReason: seriesRejectAmountUnstable,
		},
		{
			name:       "clean monthly subscription qualifies",
			charges:    []chargePoint{cp("2026-01-15", 999), cp("2026-02-15", 999), cp("2026-03-15", 999)},
			wantReason: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diag := evaluateGroup(tc.charges, "merchant", "USD")
			if diag.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q (occ=%d, medGap=%.1f, cv=%.3f, nearest=%s)",
					diag.Reason, tc.wantReason, diag.OccurrenceCount, diag.MedianGapDays, diag.IntervalCV, diag.NearestCadence)
			}
		})
	}
}

func TestEvaluateGroup_AnalyzeGroupAgreesWithReason(t *testing.T) {
	// analyzeGroup's ok must equal "reason is empty" for the same input.
	charges := []chargePoint{cp("2026-01-15", 999), cp("2026-02-15", 999), cp("2026-03-15", 999)}
	_, ok := analyzeGroup(charges, "m", "USD")
	_, diag := evaluateGroup(charges, "m", "USD")
	if ok != (diag.Reason == "") {
		t.Errorf("analyzeGroup ok=%v but evaluateGroup reason=%q — wrappers disagree", ok, diag.Reason)
	}
}

// --- recurring type inference (inferSeriesType) ---

func TestInferSeriesType(t *testing.T) {
	cases := map[string]string{
		"":                                  SeriesTypeSubscription,
		"loan_payments":                     SeriesTypeLoan,
		"loan_payments_mortgage_payment":    SeriesTypeLoan,
		"loan_payments_car_payment":         SeriesTypeLoan,
		"loan_payments_student_loan_payment": SeriesTypeLoan,
		"loan_payments_personal_loan_payment": SeriesTypeLoan,
		"loan_payments_insurance_payment":   SeriesTypeBill, // insurance is a bill, not a loan
		"loan_payments_credit_card_payment": SeriesTypeOther,
		"rent_and_utilities_rent":           SeriesTypeBill,
		"rent_and_utilities_internet_and_cable": SeriesTypeBill,
		"rent_and_utilities_gas_and_electricity": SeriesTypeBill,
		"general_services_insurance":        SeriesTypeBill,
		"entertainment_tv_and_movies":       SeriesTypeSubscription,
		"entertainment_music_and_audio":     SeriesTypeSubscription,
		"food_and_drink_coffee":             SeriesTypeSubscription, // default
		"general_merchandise_electronics":   SeriesTypeSubscription, // default
	}
	for slug, want := range cases {
		if got := inferSeriesType(slug); got != want {
			t.Errorf("inferSeriesType(%q) = %q, want %q", slug, got, want)
		}
	}
}
