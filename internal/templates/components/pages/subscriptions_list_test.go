//go:build !headless && !lite

package pages

import "testing"

func dptr(n int) *int { return &n }

// TestBuildUpcomingRenewals pins the hero rail's selection + ordering: only
// active series with a live projection (not stale, not beyond the horizon)
// earn a card, most-overdue / soonest first, with the right countdown copy.
func TestBuildUpcomingRenewals(t *testing.T) {
	t.Run("selects + orders active projected series, excludes the rest", func(t *testing.T) {
		rows := []SubscriptionRow{
			{ShortID: "soon", Status: "active", DaysUntilRenewal: dptr(7)},
			{ShortID: "overdue", Status: "active", DaysUntilRenewal: dptr(-5)},
			{ShortID: "today", Status: "active", DaysUntilRenewal: dptr(0)},
			{ShortID: "noproj", Status: "active", DaysUntilRenewal: nil},                          // excluded: no projection
			{ShortID: "stale", Status: "active", DaysUntilRenewal: dptr(-60), RenewalTone: "error"}, // excluded: stale
			{ShortID: "far", Status: "active", DaysUntilRenewal: dptr(90)},                         // excluded: beyond horizon
			{ShortID: "paused", Status: "paused", DaysUntilRenewal: dptr(3)},                       // excluded: not active
		}
		cards := BuildUpcomingRenewals(rows)
		gotIDs := make([]string, len(cards))
		for i, c := range cards {
			gotIDs[i] = c.ShortID
		}
		want := []string{"overdue", "today", "soon"}
		if len(gotIDs) != len(want) {
			t.Fatalf("got %v, want %v", gotIDs, want)
		}
		for i := range want {
			if gotIDs[i] != want[i] {
				t.Errorf("card %d = %q, want %q (order %v)", i, gotIDs[i], want[i], gotIDs)
			}
		}
	})

	t.Run("countdown labels + urgency", func(t *testing.T) {
		cases := []struct {
			days       int
			wantLabel  string
			wantUrgent string
		}{
			{-5, "5d overdue", "overdue"},
			{0, "Due today", "today"},
			{1, "Tomorrow", "soon"},
			{4, "in 4d", "soon"},
			{20, "in 20d", "later"},
		}
		for _, tc := range cases {
			cards := BuildUpcomingRenewals([]SubscriptionRow{{Status: "active", DaysUntilRenewal: dptr(tc.days)}})
			if len(cards) != 1 {
				t.Fatalf("days=%d: want 1 card, got %d", tc.days, len(cards))
			}
			if cards[0].CountLabel != tc.wantLabel || cards[0].Urgency != tc.wantUrgent {
				t.Errorf("days=%d = (%q,%q), want (%q,%q)", tc.days, cards[0].CountLabel, cards[0].Urgency, tc.wantLabel, tc.wantUrgent)
			}
		}
	})
}

// TestBuildMonthlyComposition pins the spend-composition bar: single-currency
// active monthly-equivalent, descending by amount, the tail folded into
// "Other", and no data across mixed currencies.
func TestBuildMonthlyComposition(t *testing.T) {
	t.Run("sums single currency, descending, with shares", func(t *testing.T) {
		rows := []SubscriptionRow{
			{Status: "active", HasAmount: true, Currency: "USD", MonthlyEquiv: 10, Name: "B"},
			{Status: "active", HasAmount: true, Currency: "USD", MonthlyEquiv: 30, Name: "A"},
			{Status: "active", HasAmount: false, Currency: "USD", MonthlyEquiv: 99, Name: "skip"}, // no amount
			{Status: "paused", HasAmount: true, Currency: "USD", MonthlyEquiv: 50, Name: "paused"}, // not active
		}
		comp := BuildMonthlyComposition(rows)
		if !comp.HasData || comp.Currency != "USD" || comp.Total != 40 {
			t.Fatalf("got (has=%v cur=%q total=%v), want (true USD 40)", comp.HasData, comp.Currency, comp.Total)
		}
		if len(comp.Segments) != 2 || comp.Segments[0].Label != "A" || comp.Segments[1].Label != "B" {
			t.Fatalf("segments = %+v, want [A B]", comp.Segments)
		}
		if comp.Segments[0].Percent != 75 {
			t.Errorf("top share = %v, want 75", comp.Segments[0].Percent)
		}
	})

	t.Run("folds the tail beyond top-N into Other", func(t *testing.T) {
		rows := []SubscriptionRow{
			{Status: "active", HasAmount: true, Currency: "USD", MonthlyEquiv: 50, Name: "n1"},
			{Status: "active", HasAmount: true, Currency: "USD", MonthlyEquiv: 40, Name: "n2"},
			{Status: "active", HasAmount: true, Currency: "USD", MonthlyEquiv: 30, Name: "n3"},
			{Status: "active", HasAmount: true, Currency: "USD", MonthlyEquiv: 20, Name: "n4"},
			{Status: "active", HasAmount: true, Currency: "USD", MonthlyEquiv: 6, Name: "n5"},
			{Status: "active", HasAmount: true, Currency: "USD", MonthlyEquiv: 4, Name: "n6"},
		}
		comp := BuildMonthlyComposition(rows)
		if len(comp.Segments) != monthlyCompositionTopN+1 {
			t.Fatalf("want %d segments, got %d", monthlyCompositionTopN+1, len(comp.Segments))
		}
		last := comp.Segments[len(comp.Segments)-1]
		if !last.IsOther || last.Amount != 10 {
			t.Errorf("Other segment = %+v, want IsOther+amount 10", last)
		}
	})

	t.Run("mixed currencies yield no data", func(t *testing.T) {
		rows := []SubscriptionRow{
			{Status: "active", HasAmount: true, Currency: "USD", MonthlyEquiv: 10, Name: "a"},
			{Status: "active", HasAmount: true, Currency: "EUR", MonthlyEquiv: 5, Name: "b"},
		}
		if comp := BuildMonthlyComposition(rows); comp.HasData {
			t.Errorf("want no data across currencies, got %+v", comp)
		}
	})
}

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
