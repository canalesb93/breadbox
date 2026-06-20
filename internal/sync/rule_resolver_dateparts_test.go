//go:build !lite

package sync

import (
	"testing"
	"time"
)

func fptr(f float64) *float64 { return &f }

func TestEvaluate_AmountToleranceOps(t *testing.T) {
	tctx := TransactionContext{Amount: 15.49}
	cases := []struct {
		cond *Condition
		want bool
	}{
		{&Condition{Field: "amount", Op: "approx", Value: 15.49, Tolerance: fptr(0.5)}, true},
		{&Condition{Field: "amount", Op: "approx", Value: 14.00, Tolerance: fptr(0.5)}, false},
		{&Condition{Field: "amount", Op: "approx", Value: 15.49}, false}, // nil tolerance → no match
		{&Condition{Field: "amount", Op: "between", Min: fptr(15), Max: fptr(16)}, true},
		{&Condition{Field: "amount", Op: "between", Min: fptr(16), Max: fptr(20)}, false},
		{&Condition{Field: "amount", Op: "between", Max: fptr(16)}, false}, // nil min → no match
	}
	for _, tc := range cases {
		cc := mustCompile(t, tc.cond)
		if got := evaluateCondition(cc, tctx); got != tc.want {
			t.Errorf("amount %s: got %v want %v", tc.cond.Op, got, tc.want)
		}
	}
}

func TestEvaluate_DateParts(t *testing.T) {
	d := time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC)
	tctx := TransactionContext{Date: d}
	cases := []struct {
		name string
		cond *Condition
		want bool
	}{
		{"day_of_month eq", &Condition{Field: "day_of_month", Op: "eq", Value: 15}, true},
		{"month eq", &Condition{Field: "month", Op: "eq", Value: 4}, true},
		{"day_of_week eq", &Condition{Field: "day_of_week", Op: "eq", Value: int(d.Weekday())}, true},
		{"day_of_year eq", &Condition{Field: "day_of_year", Op: "eq", Value: d.YearDay()}, true},
		{"day_of_month approx", &Condition{Field: "day_of_month", Op: "approx", Value: 14, Tolerance: fptr(3)}, true},
		{"month between", &Condition{Field: "month", Op: "between", Min: fptr(3), Max: fptr(6)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc := mustCompile(t, tc.cond)
			if got := evaluateCondition(cc, tctx); got != tc.want {
				t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestEvaluate_DatePartsZeroDate(t *testing.T) {
	tctx := TransactionContext{}
	for _, f := range []string{"day_of_month", "month", "day_of_week", "day_of_year"} {
		cc := mustCompile(t, &Condition{Field: f, Op: "eq", Value: 1})
		if evaluateCondition(cc, tctx) {
			t.Errorf("%s on zero date should not match", f)
		}
	}
}

func TestEvaluate_DayOfMonthCyclicClamp(t *testing.T) {
	cases := []struct {
		name      string
		date      time.Time
		target    float64
		tolerance float64
		want      bool
	}{
		{"feb28 target31 clamps", time.Date(2026, time.February, 28, 0, 0, 0, 0, time.UTC), 31, 0, true},
		{"leap feb29 target31 clamps", time.Date(2024, time.February, 29, 0, 0, 0, 0, time.UTC), 31, 0, true},
		{"apr30 target31 clamps", time.Date(2026, time.April, 30, 0, 0, 0, 0, time.UTC), 31, 0, true},
		{"jan31 target1 wraps", time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC), 1, 1, true},
		{"jan15 target1 far", time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC), 1, 2, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc := mustCompile(t, &Condition{Field: "day_of_month", Op: "approx", Value: tc.target, Tolerance: fptr(tc.tolerance)})
			if got := evaluateCondition(cc, TransactionContext{Date: tc.date}); got != tc.want {
				t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestEvaluate_RecurrenceComposition is the subscription-style rule the whole
// feature exists to express: amount ≈ X ± Y AND day_of_month ≈ D ± N.
func TestEvaluate_RecurrenceComposition(t *testing.T) {
	cc := mustCompile(t, &Condition{And: []Condition{
		{Field: "amount", Op: "approx", Value: 15.49, Tolerance: fptr(0.5)},
		{Field: "day_of_month", Op: "approx", Value: 14, Tolerance: fptr(3)},
	}})
	hit := TransactionContext{Amount: 15.49, Date: time.Date(2026, time.May, 13, 0, 0, 0, 0, time.UTC)}
	if !evaluateCondition(cc, hit) {
		t.Errorf("expected recurrence rule to match")
	}
	miss := TransactionContext{Amount: 15.49, Date: time.Date(2026, time.May, 25, 0, 0, 0, 0, time.UTC)}
	if evaluateCondition(cc, miss) {
		t.Errorf("did not expect match for out-of-window day")
	}
}
