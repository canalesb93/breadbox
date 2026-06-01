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
