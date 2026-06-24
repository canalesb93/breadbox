//go:build integration && !lite

// Retroactive-apply coverage for the flag / unflag rule actions (rules-as-
// universal-substrate, P1). Both retroactive entry points must materialize a
// flag: the single-rule path (ApplyRuleRetroactively) and the bulk path
// (ApplyAllRulesRetroactively, which flows through the shared
// applyRetroTxnIntent). unflag must clear flagged_at.
package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// queryFlagged returns whether the transaction's flagged_at is non-NULL.
func queryFlagged(t *testing.T, pool *pgxpool.Pool, ctx context.Context, txnID pgtype.UUID) bool {
	t.Helper()
	var at pgtype.Timestamptz
	if err := pool.QueryRow(ctx, `SELECT flagged_at FROM transactions WHERE id = $1`, txnID).Scan(&at); err != nil {
		t.Fatalf("query flagged_at: %v", err)
	}
	return at.Valid
}

// TestApplyRuleRetroactively_Flag — single-rule retroactive apply sets flagged_at.
func TestApplyRuleRetroactively_Flag(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	target := testutil.MustCreateTransaction(t, queries, acctID, "txn_retroflag", "RetroFlag Charge", 5000, "2025-02-01")

	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Flag retro",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "RetroFlag"},
		Actions:    []service.RuleAction{{Type: "flag"}},
		Priority:   10,
		Actor:      service.Actor{Type: "system", Name: "test"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	if _, err := svc.ApplyRuleRetroactively(ctx, rule.ID); err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}
	if !queryFlagged(t, pool, ctx, target.ID) {
		t.Errorf("single-rule retro flag: expected flagged_at set, got NULL")
	}
}

// TestApplyAllRulesRetroactively_Flag — bulk retroactive apply sets flagged_at
// (the shared applier path).
func TestApplyAllRulesRetroactively_Flag(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	target := testutil.MustCreateTransaction(t, queries, acctID, "txn_bulkflag", "BulkFlag Charge", 7500, "2025-02-02")
	other := testutil.MustCreateTransaction(t, queries, acctID, "txn_nomatch", "Ordinary Coffee", 400, "2025-02-03")

	if _, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Flag bulk",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "BulkFlag"},
		Actions:    []service.RuleAction{{Type: "flag"}},
		Priority:   10,
		Actor:      service.Actor{Type: "system", Name: "test"},
	}); err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	if _, err := svc.ApplyAllRulesRetroactively(ctx); err != nil {
		t.Fatalf("ApplyAllRulesRetroactively: %v", err)
	}
	if !queryFlagged(t, pool, ctx, target.ID) {
		t.Errorf("bulk retro flag: expected flagged_at set on matched row, got NULL")
	}
	if queryFlagged(t, pool, ctx, other.ID) {
		t.Errorf("bulk retro flag: non-matching row should remain unflagged")
	}
}

// TestApplyRuleRetroactively_Unflag — retroactive apply clears a pre-set flag.
func TestApplyRuleRetroactively_Unflag(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	target := testutil.MustCreateTransaction(t, queries, acctID, "txn_retrounflag", "RetroUnflag Charge", 5000, "2025-02-04")
	if _, err := pool.Exec(ctx, `UPDATE transactions SET flagged_at = NOW() WHERE id = $1`, target.ID); err != nil {
		t.Fatalf("pre-flag: %v", err)
	}

	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Unflag retro",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "RetroUnflag"},
		Actions:    []service.RuleAction{{Type: "unflag"}},
		Priority:   10,
		Actor:      service.Actor{Type: "system", Name: "test"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	if _, err := svc.ApplyRuleRetroactively(ctx, rule.ID); err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}
	if queryFlagged(t, pool, ctx, target.ID) {
		t.Errorf("retro unflag: expected flagged_at cleared, still set")
	}
}
