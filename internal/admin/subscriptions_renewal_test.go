//go:build !headless && !lite

package admin

import (
	"testing"

	"breadbox/internal/service"
)

// TestSubscriptionRenewal covers the renewal-health → attention-chip mapping
// that drives the /subscriptions ledger chip. Only due_soon / overdue / stale
// earn a chip; comfortably-renewing and no-projection series stay quiet.
func TestSubscriptionRenewal(t *testing.T) {
	days := func(n int) *int { return &n }

	cases := []struct {
		name      string
		health    string
		days      *int
		wantLabel string
		wantTone  string
	}{
		{"active stays quiet", service.SeriesHealthActive, days(30), "", ""},
		{"unknown stays quiet", service.SeriesHealthUnknown, nil, "", ""},
		{"empty (non-active) stays quiet", "", nil, "", ""},
		{"due in 3 days", service.SeriesHealthDueSoon, days(3), "Renews in 3d", "info"},
		{"due tomorrow", service.SeriesHealthDueSoon, days(1), "Due tomorrow", "info"},
		{"due today", service.SeriesHealthDueSoon, days(0), "Due today", "info"},
		{"overdue 5 days", service.SeriesHealthOverdue, days(-5), "5d overdue", "warning"},
		{"stale → likely cancelled", service.SeriesHealthStale, days(-60), "Likely cancelled", "error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := service.SeriesResponse{RenewalHealth: tc.health, DaysUntilRenewal: tc.days}
			label, tone := subscriptionRenewal(s)
			if label != tc.wantLabel || tone != tc.wantTone {
				t.Errorf("subscriptionRenewal(%q, %v) = (%q, %q), want (%q, %q)",
					tc.health, tc.days, label, tone, tc.wantLabel, tc.wantTone)
			}
		})
	}
}
