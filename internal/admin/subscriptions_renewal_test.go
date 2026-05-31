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
		typ       string
		wantLabel string
		wantTone  string
	}{
		{"active stays quiet", service.SeriesHealthActive, days(30), service.SeriesTypeSubscription, "", ""},
		{"unknown stays quiet", service.SeriesHealthUnknown, nil, service.SeriesTypeSubscription, "", ""},
		{"empty (non-active) stays quiet", "", nil, "", "", ""},
		{"due in 3 days", service.SeriesHealthDueSoon, days(3), service.SeriesTypeSubscription, "Renews in 3d", "info"},
		{"due tomorrow", service.SeriesHealthDueSoon, days(1), service.SeriesTypeSubscription, "Due tomorrow", "info"},
		{"due today", service.SeriesHealthDueSoon, days(0), service.SeriesTypeSubscription, "Due today", "info"},
		{"overdue 5 days", service.SeriesHealthOverdue, days(-5), service.SeriesTypeSubscription, "5d overdue", "warning"},
		{"stale subscription → likely cancelled", service.SeriesHealthStale, days(-60), service.SeriesTypeSubscription, "Likely cancelled", "error"},
		{"stale loan → lapsed", service.SeriesHealthStale, days(-60), service.SeriesTypeLoan, "Lapsed?", "error"},
		{"stale bill → lapsed", service.SeriesHealthStale, days(-45), service.SeriesTypeBill, "Lapsed?", "error"},
		{"stale other → likely cancelled", service.SeriesHealthStale, days(-90), service.SeriesTypeOther, "Likely cancelled", "error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := service.SeriesResponse{RenewalHealth: tc.health, DaysUntilRenewal: tc.days, Type: tc.typ}
			label, tone := subscriptionRenewal(s)
			if label != tc.wantLabel || tone != tc.wantTone {
				t.Errorf("subscriptionRenewal(health=%q, type=%q) = (%q, %q), want (%q, %q)",
					tc.health, tc.typ, label, tone, tc.wantLabel, tc.wantTone)
			}
		})
	}
}

func TestRecurringTypeLabel(t *testing.T) {
	cases := map[string]string{
		service.SeriesTypeSubscription: "Subscription",
		service.SeriesTypeBill:         "Bill",
		service.SeriesTypeLoan:         "Loan",
		service.SeriesTypeOther:        "Other",
		"":                             "Subscription", // safe default
	}
	for in, want := range cases {
		if got := recurringTypeLabel(in); got != want {
			t.Errorf("recurringTypeLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
