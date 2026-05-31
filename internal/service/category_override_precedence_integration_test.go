//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// TestCategoryOverridePrecedence proves the user > agent > rule write precedence
// through the universal update_transactions path (PR 2b):
//   - an agent write fills a 'none' row and stamps 'agent';
//   - an agent write re-categorizes its own 'agent' row;
//   - a user write overrides an 'agent' row and stamps 'user' (sacred);
//   - an agent write is SKIPPED on a 'user'-locked row — category untouched,
//     status "skipped" — while tags in the same op still apply.
func TestCategoryOverridePrecedence(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	testutil.MustCreateCategory(t, queries, "food_and_drink_groceries", "Groceries")
	testutil.MustCreateCategory(t, queries, "food_and_drink_coffee", "Coffee")
	txn := testutil.MustCreateTransaction(t, queries, acctID, "txn_prec", "Merchant", 1000, "2026-03-01")
	id := txn.ShortID

	agent := service.Actor{Type: "agent", ID: "agent-1", Name: "Routine Reviewer"}
	user := service.Actor{Type: "user", ID: "user-1", Name: "Ricardo"}

	categorize := func(actor service.Actor, slug string, tags ...string) service.UpdateTransactionsResult {
		t.Helper()
		op := service.UpdateTransactionsOp{TransactionID: id, CategorySlug: strPtr(slug)}
		for _, tg := range tags {
			op.TagsToAdd = append(op.TagsToAdd, service.UpdateTransactionsTagOp{Slug: tg})
		}
		res, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
			Operations: []service.UpdateTransactionsOp{op}, Actor: actor,
		})
		if err != nil {
			t.Fatalf("UpdateTransactions: %v", err)
		}
		return res[0]
	}
	assertState := func(wantOverride, wantSlug string) {
		t.Helper()
		got, err := svc.GetTransaction(ctx, id)
		if err != nil {
			t.Fatalf("GetTransaction: %v", err)
		}
		if got.CategoryOverride != wantOverride {
			t.Fatalf("category_override = %q, want %q", got.CategoryOverride, wantOverride)
		}
		if got.Category == nil || got.Category.Slug == nil || *got.Category.Slug != wantSlug {
			t.Fatalf("category = %+v, want slug %q", got.Category, wantSlug)
		}
	}

	// 1. Agent fills a 'none' row -> 'agent'.
	if r := categorize(agent, "food_and_drink_groceries"); r.Status != "ok" {
		t.Fatalf("agent fill status = %q, want ok", r.Status)
	}
	assertState(service.CategoryOverrideAgent, "food_and_drink_groceries")

	// 2. Agent re-categorizes its own 'agent' row.
	if r := categorize(agent, "food_and_drink_coffee"); r.Status != "ok" {
		t.Fatalf("agent recat status = %q, want ok", r.Status)
	}
	assertState(service.CategoryOverrideAgent, "food_and_drink_coffee")

	// 3. User overrides the agent's category -> 'user' (sacred).
	if r := categorize(user, "food_and_drink_groceries"); r.Status != "ok" {
		t.Fatalf("user override status = %q, want ok", r.Status)
	}
	assertState(service.CategoryOverrideUser, "food_and_drink_groceries")

	// 4. Agent write on the user-locked row is SKIPPED — category untouched —
	//    but a tag in the same op still applies.
	r := categorize(agent, "food_and_drink_coffee", "agent-touched")
	if r.Status != "skipped" {
		t.Fatalf("agent-vs-user status = %q, want skipped", r.Status)
	}
	assertState(service.CategoryOverrideUser, "food_and_drink_groceries") // unchanged
	got, err := svc.GetTransaction(ctx, id)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	var tagged bool
	for _, tg := range got.Tags {
		if tg == "agent-touched" {
			tagged = true
		}
	}
	if !tagged {
		t.Fatalf("tag from the skipped op did not apply; tags=%v", got.Tags)
	}
}
