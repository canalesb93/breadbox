//go:build !lite

package service

import (
	"testing"
	"time"
)

func fptr(f float64) *float64 { return &f }

// --- Numeric tolerance operators: approx / between --------------------------

func TestValidateCondition_NumericToleranceOps(t *testing.T) {
	cases := []struct {
		name    string
		cond    Condition
		wantErr bool
	}{
		{"approx ok", Condition{Field: "amount", Op: "approx", Value: 15.49, Tolerance: fptr(0.5)}, false},
		{"approx zero tolerance ok", Condition{Field: "amount", Op: "approx", Value: 10, Tolerance: fptr(0)}, false},
		{"approx missing tolerance", Condition{Field: "amount", Op: "approx", Value: 15.49}, true},
		{"approx negative tolerance", Condition{Field: "amount", Op: "approx", Value: 15.49, Tolerance: fptr(-1)}, true},
		{"approx non-numeric value", Condition{Field: "amount", Op: "approx", Value: "x", Tolerance: fptr(1)}, true},
		{"between ok", Condition{Field: "amount", Op: "between", Min: fptr(1), Max: fptr(5)}, false},
		{"between equal bounds ok", Condition{Field: "amount", Op: "between", Min: fptr(3), Max: fptr(3)}, false},
		{"between missing max", Condition{Field: "amount", Op: "between", Min: fptr(1)}, true},
		{"between missing min", Condition{Field: "amount", Op: "between", Max: fptr(5)}, true},
		{"between min>max", Condition{Field: "amount", Op: "between", Min: fptr(5), Max: fptr(1)}, true},
		{"approx on date-part ok", Condition{Field: "day_of_month", Op: "approx", Value: 14, Tolerance: fptr(3)}, false},
		{"between on date-part ok", Condition{Field: "day_of_month", Op: "between", Min: fptr(1), Max: fptr(5)}, false},
		{"date-part string op rejected", Condition{Field: "month", Op: "contains", Value: "2"}, true},
		{"date-part eq numeric ok", Condition{Field: "month", Op: "eq", Value: 4}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateCondition(tc.cond); (err != nil) != tc.wantErr {
				t.Errorf("ValidateCondition(%+v) err=%v wantErr=%v", tc.cond, err, tc.wantErr)
			}
		})
	}
}

func TestEvaluateCondition_AmountApproxBetween(t *testing.T) {
	tctx := TransactionContext{Amount: 15.49}
	cases := []struct {
		cond Condition
		want bool
	}{
		{Condition{Field: "amount", Op: "approx", Value: 15.49, Tolerance: fptr(0.5)}, true},
		{Condition{Field: "amount", Op: "approx", Value: 15.00, Tolerance: fptr(0.5)}, true},
		{Condition{Field: "amount", Op: "approx", Value: 14.00, Tolerance: fptr(0.5)}, false},
		{Condition{Field: "amount", Op: "approx", Value: 16.0, Tolerance: fptr(0)}, false},
		{Condition{Field: "amount", Op: "between", Min: fptr(15), Max: fptr(16)}, true},
		{Condition{Field: "amount", Op: "between", Min: fptr(15.49), Max: fptr(15.49)}, true},
		{Condition{Field: "amount", Op: "between", Min: fptr(16), Max: fptr(20)}, false},
	}
	for _, tc := range cases {
		cc := mustCompileSvc(t, tc.cond)
		if got := EvaluateCondition(cc, tctx); got != tc.want {
			t.Errorf("amount %s %v: got %v want %v", tc.cond.Op, tc.cond.Value, got, tc.want)
		}
	}
}

// --- Date-part fields --------------------------------------------------------

func TestEvaluateCondition_DateParts(t *testing.T) {
	// 2026-04-15 is a Wednesday; day-of-year 105 (2026 is not a leap year).
	d := time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC)
	tctx := TransactionContext{Date: d}
	cases := []struct {
		name string
		cond Condition
		want bool
	}{
		{"day_of_month eq", Condition{Field: "day_of_month", Op: "eq", Value: 15}, true},
		{"day_of_month neq", Condition{Field: "day_of_month", Op: "eq", Value: 16}, false},
		{"month eq", Condition{Field: "month", Op: "eq", Value: 4}, true},
		{"month between", Condition{Field: "month", Op: "between", Min: fptr(3), Max: fptr(6)}, true},
		{"day_of_week eq", Condition{Field: "day_of_week", Op: "eq", Value: int(d.Weekday())}, true},
		{"day_of_week wrong", Condition{Field: "day_of_week", Op: "eq", Value: (int(d.Weekday()) + 1) % 7}, false},
		{"day_of_year eq", Condition{Field: "day_of_year", Op: "eq", Value: d.YearDay()}, true},
		{"day_of_month approx in window", Condition{Field: "day_of_month", Op: "approx", Value: 14, Tolerance: fptr(3)}, true},
		{"day_of_month approx out of window", Condition{Field: "day_of_month", Op: "approx", Value: 1, Tolerance: fptr(3)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc := mustCompileSvc(t, tc.cond)
			if got := EvaluateCondition(cc, tctx); got != tc.want {
				t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestEvaluateCondition_DatePartsZeroDate(t *testing.T) {
	tctx := TransactionContext{} // zero Date
	for _, f := range []string{"day_of_month", "month", "day_of_week", "day_of_year"} {
		cc := mustCompileSvc(t, Condition{Field: f, Op: "eq", Value: 1})
		if EvaluateCondition(cc, tctx) {
			t.Errorf("%s on zero date should not match", f)
		}
	}
}

// TestDayOfMonthApproxCyclicClamp exercises the cyclic + clamped boundary
// behavior directly: short months, 30/31-day months, leap February, and the
// 1↔last-day wrap.
func TestDayOfMonthApproxCyclicClamp(t *testing.T) {
	cases := []struct {
		name      string
		date      time.Time
		target    float64
		tolerance float64
		want      bool
	}{
		// Clamp: "the 31st" lands on the last day of short months.
		{"feb28 target31 tol0 clamps to 28", time.Date(2026, time.February, 28, 0, 0, 0, 0, time.UTC), 31, 0, true},
		{"feb27 target31 tol0 clamps to 28 (miss)", time.Date(2026, time.February, 27, 0, 0, 0, 0, time.UTC), 31, 0, false},
		{"leap feb29 target31 tol0 clamps to 29", time.Date(2024, time.February, 29, 0, 0, 0, 0, time.UTC), 31, 0, true},
		{"leap feb28 target31 tol1 within clamp", time.Date(2024, time.February, 28, 0, 0, 0, 0, time.UTC), 31, 1, true},
		{"apr30 target31 tol0 clamps to 30", time.Date(2026, time.April, 30, 0, 0, 0, 0, time.UTC), 31, 0, true},
		{"jan31 target31 tol0 exact", time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC), 31, 0, true},
		// Cyclic wrap: 1st and the month's last day are 1 apart.
		{"jan31 target1 tol1 wraps", time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC), 1, 1, true},
		{"jan31 target1 tol0 no wrap match", time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC), 1, 0, false},
		{"feb1 target28(clamped) tol2 wraps", time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC), 28, 2, true},
		{"feb1 target31(clamp28) tol2 wraps", time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC), 31, 2, true},
		// Non-adjacent stays a miss (distance is the smaller of direct/wrap).
		{"jan15 target1 tol2 far", time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC), 1, 2, false},
		{"jan2 target1 tol1 direct", time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC), 1, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc := mustCompileSvc(t, Condition{Field: "day_of_month", Op: "approx", Value: tc.target, Tolerance: fptr(tc.tolerance)})
			if got := EvaluateCondition(cc, TransactionContext{Date: tc.date}); got != tc.want {
				t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestDayOfMonthApproxMatch_Unit covers the pure helper independent of date
// construction, including degenerate inputs.
func TestDayOfMonthApproxMatch_Unit(t *testing.T) {
	cases := []struct {
		actual, target int
		tol            float64
		monthLen       int
		want           bool
	}{
		{28, 31, 0, 28, true},  // clamp target to month length
		{1, 31, 1, 28, true},   // clamp + wrap (Feb: 28 and 1 are 1 apart)
		{15, 1, 2, 31, false},  // far
		{1, 1, 0, 31, true},    // exact
		{31, 1, 1, 31, true},   // wrap on 31-day month
		{5, 5, 0, 0, false},    // degenerate month length
	}
	for _, tc := range cases {
		got := dayOfMonthApproxMatch(tc.actual, tc.target, tc.tol, tc.monthLen)
		if got != tc.want {
			t.Errorf("dayOfMonthApproxMatch(%d,%d,%v,%d)=%v want %v", tc.actual, tc.target, tc.tol, tc.monthLen, got, tc.want)
		}
	}
}

// TestComposedRecurrenceCondition mirrors the canonical subscription rule:
// amount ≈ X ± Y AND day_of_month ≈ D ± N.
func TestComposedRecurrenceCondition(t *testing.T) {
	cond := Condition{And: []Condition{
		{Field: "amount", Op: "approx", Value: 15.49, Tolerance: fptr(0.5)},
		{Field: "day_of_month", Op: "approx", Value: 14, Tolerance: fptr(3)},
	}}
	cc := mustCompileSvc(t, cond)

	match := TransactionContext{Amount: 15.30, Date: time.Date(2026, time.March, 16, 0, 0, 0, 0, time.UTC)}
	if !EvaluateCondition(cc, match) {
		t.Errorf("expected recurrence condition to match the in-window charge")
	}
	wrongAmount := TransactionContext{Amount: 99.0, Date: time.Date(2026, time.March, 16, 0, 0, 0, 0, time.UTC)}
	if EvaluateCondition(cc, wrongAmount) {
		t.Errorf("did not expect a match for the wrong amount")
	}
	wrongDay := TransactionContext{Amount: 15.49, Date: time.Date(2026, time.March, 25, 0, 0, 0, 0, time.UTC)}
	if EvaluateCondition(cc, wrongDay) {
		t.Errorf("did not expect a match for the out-of-window day")
	}
}
