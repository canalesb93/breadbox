//go:build !headless && !lite

package admin

import (
	"testing"

	"breadbox/internal/service"
)

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

// TestSubscriptionRow_PriceChanged verifies the ledger price-change flag is
// derived from detection_signals.amount_branch == "monotonic_drift".
func TestSubscriptionRow_PriceChanged(t *testing.T) {
	empty := map[string]string{}
	drift := subscriptionRow(service.SeriesResponse{
		Type: service.SeriesTypeSubscription, Status: service.SeriesStatusActive,
		DetectionSignals: []byte(`{"amount_branch":"monotonic_drift"}`),
	}, empty, empty)
	if !drift.PriceChanged {
		t.Error("expected PriceChanged=true for monotonic_drift signals")
	}
	tight := subscriptionRow(service.SeriesResponse{
		Type: service.SeriesTypeSubscription, Status: service.SeriesStatusActive,
		DetectionSignals: []byte(`{"amount_branch":"tight"}`),
	}, empty, empty)
	if tight.PriceChanged {
		t.Error("expected PriceChanged=false for tight-band signals")
	}
	none := subscriptionRow(service.SeriesResponse{Type: service.SeriesTypeSubscription, Status: service.SeriesStatusActive}, empty, empty)
	if none.PriceChanged {
		t.Error("expected PriceChanged=false when there are no detection signals")
	}
}
