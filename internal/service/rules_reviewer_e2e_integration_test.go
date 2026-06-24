//go:build integration && !lite

package service_test

// Reviewer-authors-rule E2E (P5-PR2 / T5).
//
// These tests prove the doctrine's core loop end-to-end at the service layer:
// a scheduled REVIEWER (agent or user) authors a transaction rule, the next
// sync applies it retroactively, and the matching transactions resolve to the
// right surrogate entity (a recurring series or a counterparty). No fake
// sidecar is involved — the orchestrator/sidecar protocol is a separate
// concern; what's load-bearing for the doctrine is the
// rule → apply → membership pipeline, exercised here through the real
// CreateTransactionRule + ApplyAllRulesRetroactively service methods.
//
// Coverage:
//   - TestReviewerAuthorsRecurrenceRule: provider_name contains + amount approx
//     → assign_series(create_if_missing) → series minted, all members linked.
//   - TestReviewerAuthorsCounterpartyRule: provider_name contains
//     → assign_counterparty(create_if_missing) → counterparty resolved, members
//     bound (exercises the P4 action through the reviewer loop).
//   - TestReviewerAuthorsDayOfMonthRule: the recurrence idiom day_of_month
//     approx D ± N → assign_series matches only the in-window charges.
//
// Shared fixtures (newService, seedTxnFixture, findSeriesByName,
// countLinkedMembers, findCounterpartyByName, countLinkedCounterparty, fp) and
// the testutil helpers live in the sibling _test.go files of this package.

import (
	"context"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// TestReviewerAuthorsRecurrenceRule is the canonical reviewer loop: a reviewer
// sees ~monthly Netflix charges with no series, authors ONE rule on stable
// fields (provider_name + amount), and the next sync (modeled by
// ApplyAllRulesRetroactively) mints the series and links every member. The
// amount approx condition exercises the P1 numeric `approx` operator through
// the bulk retroactive path; the seeded amounts differ slightly (15.49 /
// 14.99 / 15.99) so the ± window does real work rather than an exact match.
func TestReviewerAuthorsRecurrenceRule(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// Three "Netflix" charges (provider_name contains "Netflix"), ~$15.49, on
	// different month-days — no series assigned yet.
	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX_jan", "Netflix", 1549, "2026-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX_feb", "Netflix", 1499, "2026-02-17")
	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX_mar", "Netflix", 1599, "2026-03-16")

	// Sanity: no Netflix series exists before the reviewer authors the rule.
	if s := findSeriesByName(t, svc, "Netflix"); s != nil {
		t.Fatalf("precondition: Netflix series already exists (%s)", s.ShortID)
	}

	// The reviewer authors the rule. Conditions are on STABLE fields only
	// (provider_name + amount), never mutable account_name/category/series.
	// Lowercase "netflix" exercises the case-insensitive `contains` op against
	// the seeded "Netflix" provider_name.
	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name: "Netflix subscription → series",
		Conditions: service.Condition{And: []service.Condition{
			{Field: "provider_name", Op: "contains", Value: "netflix"},
			{Field: "amount", Op: "approx", Value: 15.49, Tolerance: fp(1.0)},
		}},
		Actions: []service.RuleAction{{
			Type:            "assign_series",
			SeriesName:      "Netflix",
			CreateIfMissing: true,
		}},
		Actor: service.Actor{Type: "agent", ID: "agent-1", Name: "Transaction Reviewer"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}
	if rule.CreatedByType != "agent" {
		t.Errorf("rule created_by_type = %q, want agent", rule.CreatedByType)
	}

	// The next sync applies all enabled rules retroactively.
	if _, err := svc.ApplyAllRulesRetroactively(ctx); err != nil {
		t.Fatalf("ApplyAllRulesRetroactively: %v", err)
	}

	// Assert: a "Netflix" series now exists and all 3 transactions are members.
	series := findSeriesByName(t, svc, "Netflix")
	if series == nil {
		t.Fatal("reviewer loop did not mint a Netflix series")
	}
	if got := countLinkedMembers(t, pool, series.ID); got != 3 {
		t.Errorf("linked series members = %d, want 3", got)
	}
}

// TestReviewerAuthorsCounterpartyRule mirrors the recurrence loop but targets
// the P4 counterparty entity: the reviewer authors a rule whose action is
// assign_counterparty(create_if_missing), and the next sync resolves all
// matching transactions to a single Netflix counterparty. Proves the P4 action
// materializes through the reviewer→apply pipeline (the bulk retroactive path
// that historically dropped assign_series).
func TestReviewerAuthorsCounterpartyRule(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX_a", "Netflix", 1549, "2026-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX_b", "Netflix", 1499, "2026-02-17")
	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX_c", "Netflix", 1599, "2026-03-16")

	if cp := findCounterpartyByName(t, svc, "Netflix"); cp != nil {
		t.Fatalf("precondition: Netflix counterparty already exists (%s)", cp.ShortID)
	}

	if _, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Netflix → counterparty",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "netflix"},
		Actions: []service.RuleAction{{
			Type:             "assign_counterparty",
			CounterpartyName: "Netflix",
			CreateIfMissing:  true,
		}},
		Actor: service.Actor{Type: "agent", ID: "agent-1", Name: "Transaction Reviewer"},
	}); err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	if _, err := svc.ApplyAllRulesRetroactively(ctx); err != nil {
		t.Fatalf("ApplyAllRulesRetroactively: %v", err)
	}

	cp := findCounterpartyByName(t, svc, "Netflix")
	if cp == nil {
		t.Fatal("reviewer loop did not resolve a Netflix counterparty")
	}
	if got := countLinkedCounterparty(t, pool, cp.ID); got != 3 {
		t.Errorf("bound counterparty members = %d, want 3", got)
	}
}

// TestReviewerAuthorsDayOfMonthRule exercises the date-part recurrence idiom
// (day_of_month approx D ± N) through the reviewer loop: the reviewer encodes
// "around the 15th of the month" and the next sync links only the in-window
// charges — an off-day charge on the 28th is correctly excluded from the
// series even though it shares the merchant and amount.
func TestReviewerAuthorsDayOfMonthRule(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// In-window: days 14/15/16, within 15 ± 3.
	testutil.MustCreateTransaction(t, queries, acctID, "HULU_jan", "Hulu", 1499, "2026-01-14")
	testutil.MustCreateTransaction(t, queries, acctID, "HULU_feb", "Hulu", 1499, "2026-02-15")
	testutil.MustCreateTransaction(t, queries, acctID, "HULU_mar", "Hulu", 1499, "2026-03-16")
	// Out-of-window day: the 28th — same merchant + amount, but off-cadence.
	testutil.MustCreateTransaction(t, queries, acctID, "HULU_off", "Hulu", 1499, "2026-04-28")

	if _, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name: "Hulu mid-month → series",
		Conditions: service.Condition{And: []service.Condition{
			{Field: "provider_name", Op: "contains", Value: "hulu"},
			{Field: "day_of_month", Op: "approx", Value: 15, Tolerance: fp(3)},
		}},
		Actions: []service.RuleAction{{
			Type:            "assign_series",
			SeriesName:      "Hulu",
			CreateIfMissing: true,
		}},
		Actor: service.Actor{Type: "agent", ID: "agent-1", Name: "Transaction Reviewer"},
	}); err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	if _, err := svc.ApplyAllRulesRetroactively(ctx); err != nil {
		t.Fatalf("ApplyAllRulesRetroactively: %v", err)
	}

	series := findSeriesByName(t, svc, "Hulu")
	if series == nil {
		t.Fatal("reviewer loop did not mint a Hulu series")
	}
	// Only the 3 in-window charges link; the 28th is excluded by the date-part.
	if got := countLinkedMembers(t, pool, series.ID); got != 3 {
		t.Errorf("linked series members = %d, want 3 (off-day charge must be excluded)", got)
	}
}
