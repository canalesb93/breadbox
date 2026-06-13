//go:build !headless && !lite

package pages

import "testing"

// TestGroupSubscriptionsByStatus pins the ledger IA: rows bucket into
// Active → Paused → Ended in that order, unknown statuses sink last in
// first-seen order, row order is preserved within a group, and only the
// Active group carries a monthly subtotal — single-currency only.
func TestGroupSubscriptionsByStatus(t *testing.T) {
	t.Run("orders groups active → paused → ended and preserves row order", func(t *testing.T) {
		rows := []SubscriptionRow{
			{ShortID: "p1", Status: "paused"},
			{ShortID: "a1", Status: "active"},
			{ShortID: "c1", Status: "cancelled"},
			{ShortID: "a2", Status: "active"},
		}
		groups := GroupSubscriptionsByStatus(rows)
		if len(groups) != 3 {
			t.Fatalf("want 3 groups, got %d", len(groups))
		}
		wantOrder := []struct{ status, label string }{
			{"active", "Active"},
			{"paused", "Paused"},
			{"cancelled", "Ended"},
		}
		for i, w := range wantOrder {
			if groups[i].Status != w.status || groups[i].Label != w.label {
				t.Errorf("group %d = (%q,%q), want (%q,%q)", i, groups[i].Status, groups[i].Label, w.status, w.label)
			}
		}
		// Within the active group, incoming order (a1 before a2) is preserved.
		active := groups[0]
		if len(active.Rows) != 2 || active.Rows[0].ShortID != "a1" || active.Rows[1].ShortID != "a2" {
			t.Errorf("active group rows = %+v, want [a1 a2]", active.Rows)
		}
	})

	t.Run("empty input yields no groups", func(t *testing.T) {
		if g := GroupSubscriptionsByStatus(nil); len(g) != 0 {
			t.Errorf("want 0 groups for nil input, got %d", len(g))
		}
	})

	t.Run("unknown status sinks last with a title-cased label", func(t *testing.T) {
		rows := []SubscriptionRow{
			{Status: "weird"},
			{Status: "active"},
		}
		groups := GroupSubscriptionsByStatus(rows)
		if len(groups) != 2 {
			t.Fatalf("want 2 groups, got %d", len(groups))
		}
		if groups[0].Status != "active" {
			t.Errorf("first group = %q, want active", groups[0].Status)
		}
		if groups[1].Status != "weird" || groups[1].Label != "Weird" {
			t.Errorf("last group = (%q,%q), want (weird,Weird)", groups[1].Status, groups[1].Label)
		}
	})

	t.Run("active group sums monthly equivalent for one currency", func(t *testing.T) {
		rows := []SubscriptionRow{
			{Status: "active", HasAmount: true, Currency: "USD", MonthlyEquiv: 10},
			{Status: "active", HasAmount: true, Currency: "USD", MonthlyEquiv: 5.5},
			{Status: "active", HasAmount: false, Currency: "USD", MonthlyEquiv: 99}, // no amount, ignored
		}
		g := GroupSubscriptionsByStatus(rows)[0]
		if !g.HasSubtotal {
			t.Fatal("want HasSubtotal=true for single-currency active group")
		}
		if g.SubtotalCurrency != "USD" {
			t.Errorf("subtotal currency = %q, want USD", g.SubtotalCurrency)
		}
		if g.Subtotal != 15.5 {
			t.Errorf("subtotal = %v, want 15.5", g.Subtotal)
		}
	})

	t.Run("blank currency normalizes to USD for the single-currency check", func(t *testing.T) {
		rows := []SubscriptionRow{
			{Status: "active", HasAmount: true, Currency: "", MonthlyEquiv: 4},
			{Status: "active", HasAmount: true, Currency: "USD", MonthlyEquiv: 6},
		}
		g := GroupSubscriptionsByStatus(rows)[0]
		if !g.HasSubtotal || g.SubtotalCurrency != "USD" || g.Subtotal != 10 {
			t.Errorf("got (has=%v cur=%q sub=%v), want (true USD 10)", g.HasSubtotal, g.SubtotalCurrency, g.Subtotal)
		}
	})

	t.Run("mixed currencies suppress the subtotal", func(t *testing.T) {
		rows := []SubscriptionRow{
			{Status: "active", HasAmount: true, Currency: "USD", MonthlyEquiv: 10},
			{Status: "active", HasAmount: true, Currency: "EUR", MonthlyEquiv: 5},
		}
		g := GroupSubscriptionsByStatus(rows)[0]
		if g.HasSubtotal {
			t.Errorf("want no subtotal across currencies, got %v %q", g.Subtotal, g.SubtotalCurrency)
		}
	})

	t.Run("non-active groups never carry a subtotal", func(t *testing.T) {
		rows := []SubscriptionRow{
			{Status: "paused", HasAmount: true, Currency: "USD", MonthlyEquiv: 12},
		}
		g := GroupSubscriptionsByStatus(rows)[0]
		if g.HasSubtotal {
			t.Errorf("paused group should have no subtotal, got %v", g.Subtotal)
		}
	})
}
