//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

func fp(f float64) *float64 { return &f }

// TestApplyRuleRetroactively_RecurrenceCondition is the canonical detector-free
// subscription rule: amount ≈ X ± Y AND day_of_month ≈ D ± N → add_tag. It must
// match only the in-window charge during retroactive apply.
func TestApplyRuleRetroactively_RecurrenceCondition(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "User")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	testutil.MustCreateTag(t, queries, "subscription", "Subscription")

	// In-window: $15.49 on the 16th (within 14 ± 3 for a 31-day month).
	match := testutil.MustCreateTransaction(t, queries, acct.ID, "m_match", "Netflix", 1549, "2026-03-16")
	// Out-of-window day: $15.49 on the 25th.
	offDay := testutil.MustCreateTransaction(t, queries, acct.ID, "m_offday", "Netflix", 1549, "2026-03-25")
	// Out-of-window amount: $99.00 on the 14th.
	offAmt := testutil.MustCreateTransaction(t, queries, acct.ID, "m_offamt", "Spotify", 9900, "2026-03-14")

	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:    "Netflix subscription",
		Actions: []service.RuleAction{{Type: "add_tag", TagSlug: "subscription"}},
		Conditions: service.Condition{And: []service.Condition{
			{Field: "amount", Op: "approx", Value: 15.49, Tolerance: fp(0.5)},
			{Field: "day_of_month", Op: "approx", Value: 14, Tolerance: fp(3)},
		}},
		Priority: 10,
		Actor:    service.Actor{Type: "user", Name: "Test"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	matched, err := svc.ApplyRuleRetroactively(ctx, rule.ID)
	if err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}
	if matched != 1 {
		t.Fatalf("expected exactly 1 matched txn, got %d", matched)
	}

	tagged := func(id string) bool {
		var n int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM transaction_tags tt
			JOIN tags g ON g.id = tt.tag_id
			WHERE tt.transaction_id = $1 AND g.slug = 'subscription'`, id).Scan(&n); err != nil {
			t.Fatalf("count tags for %s: %v", id, err)
		}
		return n == 1
	}

	if !tagged(pgconv.FormatUUID(match.ID)) {
		t.Errorf("in-window charge should be tagged 'subscription'")
	}
	if tagged(pgconv.FormatUUID(offDay.ID)) {
		t.Errorf("out-of-window day should NOT be tagged")
	}
	if tagged(pgconv.FormatUUID(offAmt.ID)) {
		t.Errorf("out-of-window amount should NOT be tagged")
	}
}

// TestApplyRuleRetroactively_AmountBetweenAndMonth covers the between operator
// and the month date-part through the retroactive path.
func TestApplyRuleRetroactively_AmountBetweenAndMonth(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "User")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	testutil.MustCreateTag(t, queries, "annual", "Annual")

	// April charge in [$90,$110] → match. December charge same amount → month miss.
	apr := testutil.MustCreateTransaction(t, queries, acct.ID, "a_apr", "Insurance", 10000, "2026-04-12")
	dec := testutil.MustCreateTransaction(t, queries, acct.ID, "a_dec", "Insurance", 10000, "2026-12-12")

	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:    "Annual insurance",
		Actions: []service.RuleAction{{Type: "add_tag", TagSlug: "annual"}},
		Conditions: service.Condition{And: []service.Condition{
			{Field: "amount", Op: "between", Min: fp(90), Max: fp(110)},
			{Field: "month", Op: "eq", Value: 4},
			{Field: "day_of_month", Op: "approx", Value: 12, Tolerance: fp(2)},
		}},
		Priority: 10,
		Actor:    service.Actor{Type: "user", Name: "Test"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}
	if _, err := svc.ApplyRuleRetroactively(ctx, rule.ID); err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}

	tagged := func(id string) bool {
		var n int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM transaction_tags tt JOIN tags g ON g.id = tt.tag_id
			WHERE tt.transaction_id = $1 AND g.slug = 'annual'`, id).Scan(&n); err != nil {
			t.Fatalf("count tags: %v", err)
		}
		return n == 1
	}
	if !tagged(pgconv.FormatUUID(apr.ID)) {
		t.Errorf("April charge in range should be tagged")
	}
	if tagged(pgconv.FormatUUID(dec.ID)) {
		t.Errorf("December charge should NOT match the month=4 condition")
	}
}
