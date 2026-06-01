//go:build !lite

package service

import (
	"strings"
	"testing"
)

func TestDescribeCron(t *testing.T) {
	var s Service
	cases := []struct {
		name      string
		expr      string
		wantValid bool
		substr    string // case-insensitive substring expected in the description
	}{
		{"daily 8am", "0 8 * * *", true, "08:00 AM"},
		{"every monday 7am", "0 7 * * 1", true, "Monday"},
		{"twice weekly tue+thu 7pm", "0 19 * * 2,4", true, "Tuesday"},
		{"every 15 min", "*/15 * * * *", true, "15 minutes"},
		{"empty is invalid", "", false, "schedule"},
		{"garbage is invalid", "not a cron", false, "valid"},
		{"too few fields invalid", "0 8 *", false, "valid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			valid, desc := s.DescribeCron(c.expr)
			if valid != c.wantValid {
				t.Fatalf("DescribeCron(%q) valid=%v, want %v (desc=%q)", c.expr, valid, c.wantValid, desc)
			}
			if c.substr != "" && !strings.Contains(strings.ToLower(desc), strings.ToLower(c.substr)) {
				t.Errorf("DescribeCron(%q) = %q, want it to contain %q", c.expr, desc, c.substr)
			}
		})
	}
}

// TestShiftCronTimeFields covers the timezone-shift logic that backs
// DescribeCronInTZ. It is a pure function of (expr, deltaMinutes) so it's
// deterministic regardless of the test host's local timezone.
func TestShiftCronTimeFields(t *testing.T) {
	cases := []struct {
		name   string
		expr   string
		delta  int
		want   string
		wantOk bool
	}{
		// Daily, no midnight wrap: 08:00 +2h → 10:00, still daily.
		{"daily +2h", "0 8 * * *", 120, "0 10 * * *", true},
		// Daily, negative shift staying same day: 08:00 −3h → 05:00.
		{"daily -3h", "0 8 * * *", -180, "0 5 * * *", true},
		// Half-hour offset (e.g. UTC→IST +5:30): minute shifts too.
		{"daily +5h30", "0 8 * * *", 330, "30 13 * * *", true},
		// Weekly, wrap forward past midnight bumps the weekday: Mon 23:00
		// +2h → Tue 01:00 (dow 1 → 2).
		{"weekly wrap forward", "0 23 * * 1", 120, "0 1 * * 2", true},
		// Weekly, wrap backward before midnight: Mon 01:00 −2h → Sun 23:00
		// (dow 1 → 0).
		{"weekly wrap backward", "0 1 * * 1", -120, "0 23 * * 0", true},
		// Sunday normalization: dow 7 (Sunday) wrapping forward → Mon (1).
		{"sunday wrap", "0 23 * * 7", 120, "0 1 * * 1", true},
		// Multi-day list wraps each member: Tue,Thu 23:00 +2h → Wed,Fri.
		{"multi-dow wrap", "0 23 * * 2,4", 120, "0 1 * * 3,5", true},
		// No wrap, weekday untouched: Mon 08:00 +2h → Mon 10:00.
		{"weekly no wrap", "0 8 * * 1", 120, "0 10 * * 1", true},
		// Monthly with a wrap is not representable → fall back.
		{"monthly wrap not ok", "0 23 1 * *", 120, "", false},
		// Step minute is not a single integer → fall back.
		{"step minute not ok", "*/15 * * * *", 120, "", false},
		// Range hour is not a single integer → fall back.
		{"range hour not ok", "0 8-10 * * *", 120, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := shiftCronTimeFields(c.expr, c.delta)
			if ok != c.wantOk {
				t.Fatalf("shiftCronTimeFields(%q, %d) ok=%v, want %v (got=%q)", c.expr, c.delta, ok, c.wantOk, got)
			}
			if c.wantOk && got != c.want {
				t.Errorf("shiftCronTimeFields(%q, %d) = %q, want %q", c.expr, c.delta, got, c.want)
			}
		})
	}
}
