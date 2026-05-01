//go:build integration

package service_test

import (
	"context"
	"errors"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
)

// strPtr is a tiny helper for the *string sites in UpdateTransactionsOp.
func strPtr(s string) *string { return &s }

// TestUpdateTransactions_CompoundOp verifies set_category + add_tag + remove_tag
// + comment all run atomically per-op, with annotations written for each.
func TestUpdateTransactions_CompoundOp(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "extu1", "Costco", 8732, "2026-04-01")

	// Seed required category + needs-review tag.
	testutil.MustCreateCategory(t, queries, "food_and_drink_groceries", "Groceries")
	testutil.MustCreateTag(t, queries, "needs-review", "Needs Review")

	// Pre-attach needs-review to mimic the seeded auto-rule.
	if _, _, err := svc.AddTransactionTag(ctx, txn.ShortID, "needs-review", service.Actor{Type: "user", ID: "u1", Name: "Tester"}); err != nil {
		t.Fatalf("seed AddTransactionTag: %v", err)
	}

	// Compound op: set category, add a new tag, remove needs-review, and
	// attach a comment carrying the rationale (notes on tag actions are
	// deprecated — the comment is the audit artifact).
	results, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{{
			TransactionID: txn.ShortID,
			CategorySlug:  strPtr("food_and_drink_groceries"),
			TagsToAdd:     []service.UpdateTransactionsTagOp{{Slug: "subscription:monthly"}},
			TagsToRemove:  []service.UpdateTransactionsTagOp{{Slug: "needs-review"}},
			Comment:       strPtr("Clearly groceries — Costco run."),
		}},
		Actor: service.Actor{Type: "user", ID: "u1", Name: "Tester"},
	})
	if err != nil {
		t.Fatalf("UpdateTransactions: %v", err)
	}
	if len(results) != 1 || results[0].Status != "ok" {
		t.Fatalf("expected one ok result, got: %+v", results)
	}

	// Verify category set.
	got, err := svc.GetTransaction(ctx, txn.ShortID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if got.Category == nil || got.Category.Slug == nil || *got.Category.Slug != "food_and_drink_groceries" {
		t.Errorf("expected category food_and_drink_groceries, got %+v", got.Category)
	}
	if !got.CategoryOverride {
		t.Error("expected category_override=true after set_category")
	}

	// Verify tag state.
	tags, err := svc.ListTransactionTags(ctx, txn.ShortID)
	if err != nil {
		t.Fatalf("ListTransactionTags: %v", err)
	}
	hasSub := false
	hasReview := false
	for _, t := range tags {
		if t.Slug == "subscription:monthly" {
			hasSub = true
		}
		if t.Slug == "needs-review" {
			hasReview = true
		}
	}
	if !hasSub {
		t.Error("expected subscription:monthly tag attached")
	}
	if hasReview {
		t.Error("expected needs-review tag to have been removed")
	}

	// Verify annotations: tag_added (subscription) + tag_removed (needs-review)
	// + category_set + comment + the original tag_added from seed.
	got1 := testutil.MustCountAnnotations(t, queries, txn.ID, "tag_added")
	if got1 < 2 {
		t.Errorf("expected >= 2 tag_added annotations (seed + new), got %d", got1)
	}
	if got2 := testutil.MustCountAnnotations(t, queries, txn.ID, "tag_removed"); got2 != 1 {
		t.Errorf("expected 1 tag_removed annotation, got %d", got2)
	}
	if got3 := testutil.MustCountAnnotations(t, queries, txn.ID, "category_set"); got3 != 1 {
		t.Errorf("expected 1 category_set annotation, got %d", got3)
	}
	if got4 := testutil.MustCountAnnotations(t, queries, txn.ID, "comment"); got4 != 1 {
		t.Errorf("expected 1 comment annotation, got %d", got4)
	}
}

// TestUpdateTransactions_ContinueMode_PartialFailure verifies that a per-op
// error in continue mode does NOT undo successful items.
func TestUpdateTransactions_ContinueMode_PartialFailure(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	tx1 := testutil.MustCreateTransaction(t, queries, acctID, "tx_a", "Coffee", 500, "2026-04-01")

	testutil.MustCreateCategory(t, queries, "food_and_drink_coffee", "Coffee")

	results, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{
			{TransactionID: tx1.ShortID, CategorySlug: strPtr("food_and_drink_coffee")},
			// Bad op: nonexistent transaction.
			{TransactionID: "nonexist", CategorySlug: strPtr("food_and_drink_coffee")},
			// Another good op.
			{TransactionID: tx1.ShortID, Comment: strPtr("nice coffee")},
		},
		OnError: "continue",
		Actor:   service.Actor{Type: "user", ID: "u1", Name: "Tester"},
	})
	if err != nil {
		t.Fatalf("UpdateTransactions returned err in continue mode: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Status != "ok" {
		t.Errorf("op[0] expected ok, got %+v", results[0])
	}
	if results[1].Status != "error" {
		t.Errorf("op[1] expected error, got %+v", results[1])
	}
	if results[2].Status != "ok" {
		t.Errorf("op[2] expected ok, got %+v", results[2])
	}

	// Verify successful changes persisted.
	got, _ := svc.GetTransaction(ctx, tx1.ShortID)
	if got.Category == nil || got.Category.Slug == nil || *got.Category.Slug != "food_and_drink_coffee" {
		t.Errorf("expected category food_and_drink_coffee on tx1, got %+v", got.Category)
	}
}

// TestUpdateTransactions_AbortMode_RollsBack verifies that abort mode
// rolls back the entire batch on the first error.
func TestUpdateTransactions_AbortMode_RollsBack(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	tx1 := testutil.MustCreateTransaction(t, queries, acctID, "tx_b", "Latte", 600, "2026-04-02")

	testutil.MustCreateCategory(t, queries, "food_and_drink_coffee", "Coffee")

	_, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{
			{TransactionID: tx1.ShortID, CategorySlug: strPtr("food_and_drink_coffee")},
			// This will fail (nonexistent transaction).
			{TransactionID: "nonexist", CategorySlug: strPtr("food_and_drink_coffee")},
		},
		OnError: "abort",
		Actor:   service.Actor{Type: "user", ID: "u1", Name: "Tester"},
	})
	if err == nil {
		t.Fatal("expected abort mode to surface a top-level error")
	}

	// First op's category change should have been rolled back.
	got, _ := svc.GetTransaction(ctx, tx1.ShortID)
	if got.Category != nil && got.Category.Slug != nil && *got.Category.Slug == "food_and_drink_coffee" {
		t.Errorf("expected category change rolled back, but tx1 is still categorized: %+v", got.Category)
	}
}

// TestUpdateTransactions_TagRemoval verifies that tag removal via the
// compound op writes a tag_removed annotation and detaches the tag.
func TestUpdateTransactions_TagRemoval(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "tx_c", "Subscription", 999, "2026-04-03")

	testutil.MustCreateTag(t, queries, "needs-review", "Needs Review")
	if _, _, err := svc.AddTransactionTag(ctx, txn.ShortID, "needs-review", service.Actor{Type: "user", ID: "u1", Name: "Tester"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	results, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{{
			TransactionID: txn.ShortID,
			TagsToRemove:  []service.UpdateTransactionsTagOp{{Slug: "needs-review"}},
		}},
		Actor: service.Actor{Type: "user", ID: "u1", Name: "Tester"},
	})
	if err != nil {
		t.Fatalf("UpdateTransactions: %v", err)
	}
	if len(results) != 1 || results[0].Status != "ok" {
		t.Fatalf("expected per-op ok, got %+v", results)
	}

	tags, _ := svc.ListTransactionTags(ctx, txn.ShortID)
	for _, tg := range tags {
		if tg.Slug == "needs-review" {
			t.Error("expected needs-review tag to be removed")
		}
	}
}

// TestUpdateTransactions_ResetCategory verifies that ResetCategory clears a
// prior manual override the same way the standalone ResetTransactionCategory
// service did (it's the absorbed replacement after the MCP tool collapse).
func TestUpdateTransactions_ResetCategory(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "extReset1", "Whole Foods", 4321, "2026-04-01")

	testutil.MustCreateCategory(t, queries, "food_and_drink_groceries", "Groceries")
	testutil.MustCreateCategory(t, queries, "uncategorized", "Uncategorized")
	actor := service.Actor{Type: "user", ID: "u1", Name: "Tester"}

	// First, set an override so there's something to reset.
	if _, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{{
			TransactionID: txn.ShortID,
			CategorySlug:  strPtr("food_and_drink_groceries"),
		}},
		Actor: actor,
	}); err != nil {
		t.Fatalf("seed override: %v", err)
	}

	// Reset clears the override and drops to uncategorized.
	results, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{{
			TransactionID: txn.ShortID,
			ResetCategory: true,
			Comment:       strPtr("Letting rules re-categorize."),
		}},
		Actor: actor,
	})
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if len(results) != 1 || results[0].Status != "ok" {
		t.Fatalf("expected ok, got %+v", results)
	}

	// Verify the override flag is cleared and category is uncategorized.
	got, err := svc.GetTransaction(ctx, txn.ShortID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if got.CategoryOverride {
		t.Errorf("category_override = true, want false after reset")
	}
	if got.Category == nil || got.Category.Slug == nil || *got.Category.Slug != "uncategorized" {
		t.Errorf("expected category=uncategorized after reset, got %+v", got.Category)
	}
}

// TestUpdateTransactions_ResetAndSetMutuallyExclusive verifies the validation
// that rejects supplying both category_slug and reset_category in one op.
func TestUpdateTransactions_ResetAndSetMutuallyExclusive(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "extReset2", "Costco", 1234, "2026-04-01")

	testutil.MustCreateCategory(t, queries, "food_and_drink_groceries", "Groceries")

	results, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{{
			TransactionID: txn.ShortID,
			CategorySlug:  strPtr("food_and_drink_groceries"),
			ResetCategory: true,
		}},
		Actor: service.SystemActor(),
	})
	if err != nil {
		t.Fatalf("UpdateTransactions returned top-level error: %v", err)
	}
	if len(results) != 1 || results[0].Status != "error" {
		t.Fatalf("expected per-op error status, got %+v", results)
	}
	if results[0].Error == nil || results[0].Error.Code != "INVALID_PARAMETER" {
		t.Errorf("expected INVALID_PARAMETER, got %+v", results[0].Error)
	}
}

// TestUpdateTransactions_RejectsTooMany verifies the 50-op cap.
func TestUpdateTransactions_RejectsTooMany(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	ops := make([]service.UpdateTransactionsOp, 51)
	for i := range ops {
		ops[i] = service.UpdateTransactionsOp{TransactionID: "anything"}
	}
	_, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{Operations: ops})
	if err == nil {
		t.Fatal("expected error for 51 operations")
	}
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got %v", err)
	}
}

// TestUpdateTransactions_ResetCategoryNoOverride guards against the edge case
// where reset_category fires on a transaction that never had a manual override.
// The collapse of reset_transaction_category into update_transactions made this
// path easy to silently break: the SQL `UPDATE … SET category_override=FALSE`
// is idempotent (rows-affected stays 1 because the row exists), so we want to
// confirm the handler still drops to uncategorized AND still emits the
// category_set annotation rather than short-circuiting on the no-op flag flip.
func TestUpdateTransactions_ResetCategoryNoOverride(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "extResetNoOverride", "Trader Joes", 1500, "2026-04-04")

	testutil.MustCreateCategory(t, queries, "uncategorized", "Uncategorized")
	actor := service.Actor{Type: "user", ID: "u1", Name: "Tester"}

	// Pre-condition: brand-new transaction, no manual override, no category set.
	pre, err := svc.GetTransaction(ctx, txn.ShortID)
	if err != nil {
		t.Fatalf("GetTransaction pre: %v", err)
	}
	if pre.CategoryOverride {
		t.Fatalf("seed precondition: expected category_override=false on fresh txn, got true")
	}

	results, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{{
			TransactionID: txn.ShortID,
			ResetCategory: true,
		}},
		Actor: actor,
	})
	if err != nil {
		t.Fatalf("reset (no prior override): %v", err)
	}
	if len(results) != 1 || results[0].Status != "ok" {
		t.Fatalf("expected per-op ok, got %+v", results)
	}

	got, err := svc.GetTransaction(ctx, txn.ShortID)
	if err != nil {
		t.Fatalf("GetTransaction post: %v", err)
	}
	if got.CategoryOverride {
		t.Errorf("category_override=true after reset, want false")
	}
	if got.Category == nil || got.Category.Slug == nil || *got.Category.Slug != "uncategorized" {
		t.Errorf("expected category=uncategorized after reset, got %+v", got.Category)
	}
	// Annotation must still be written even when the override was already false —
	// the audit trail is what callers (and the timeline UI) rely on.
	if n := testutil.MustCountAnnotations(t, queries, txn.ID, "category_set"); n != 1 {
		t.Errorf("expected exactly 1 category_set annotation from the reset, got %d", n)
	}
}

// TestUpdateTransactions_ResetCombinedWithTagAndComment guards the order of
// operations inside runUpdateOpInTx now that reset_category shares the
// compound op. The contract is: reset runs first, then tag adds/removes, then
// the comment — and every per-row change writes its own annotation. A
// regression that ran reset *after* the tag write would leave the category in
// the wrong state; one that swallowed the comment annotation would break the
// review-loop audit trail.
func TestUpdateTransactions_ResetCombinedWithTagAndComment(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "extResetCombined", "Costco", 9876, "2026-04-05")

	testutil.MustCreateCategory(t, queries, "food_and_drink_groceries", "Groceries")
	testutil.MustCreateCategory(t, queries, "uncategorized", "Uncategorized")
	actor := service.Actor{Type: "user", ID: "u1", Name: "Tester"}

	// Set an override so the reset has something to clear.
	if _, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{{
			TransactionID: txn.ShortID,
			CategorySlug:  strPtr("food_and_drink_groceries"),
		}},
		Actor: actor,
	}); err != nil {
		t.Fatalf("seed override: %v", err)
	}

	// Compound: reset_category + add a new tag + comment in a single op.
	results, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{{
			TransactionID: txn.ShortID,
			ResetCategory: true,
			TagsToAdd:     []service.UpdateTransactionsTagOp{{Slug: "rethink"}},
			Comment:       strPtr("Reverting categorization — merchant ambiguous."),
		}},
		Actor: actor,
	})
	if err != nil {
		t.Fatalf("compound reset+tag+comment: %v", err)
	}
	if len(results) != 1 || results[0].Status != "ok" {
		t.Fatalf("expected per-op ok, got %+v", results)
	}

	// Final state: override cleared, dropped to uncategorized, new tag attached.
	got, err := svc.GetTransaction(ctx, txn.ShortID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if got.CategoryOverride {
		t.Error("category_override=true after reset, want false")
	}
	if got.Category == nil || got.Category.Slug == nil || *got.Category.Slug != "uncategorized" {
		t.Errorf("expected uncategorized after reset, got %+v", got.Category)
	}
	tags, err := svc.ListTransactionTags(ctx, txn.ShortID)
	if err != nil {
		t.Fatalf("ListTransactionTags: %v", err)
	}
	hasRethink := false
	for _, tg := range tags {
		if tg.Slug == "rethink" {
			hasRethink = true
		}
	}
	if !hasRethink {
		t.Error("expected 'rethink' tag attached after compound op")
	}

	// Annotation correctness: every per-row change in the compound op should
	// have written exactly one annotation. Two category_set (one from the seed
	// override, one from the reset), one tag_added, one comment.
	if n := testutil.MustCountAnnotations(t, queries, txn.ID, "category_set"); n != 2 {
		t.Errorf("expected 2 category_set annotations (seed override + reset), got %d", n)
	}
	if n := testutil.MustCountAnnotations(t, queries, txn.ID, "tag_added"); n != 1 {
		t.Errorf("expected 1 tag_added annotation, got %d", n)
	}
	if n := testutil.MustCountAnnotations(t, queries, txn.ID, "comment"); n != 1 {
		t.Errorf("expected 1 comment annotation, got %d", n)
	}
}

// TestApplyRulesRespectsOverrideFromUpdateTransactions covers the cross-cutting
// invariant that update_transactions' category_slug write sets
// category_override=true, and that retroactive ApplyAllRulesRetroactively
// honors that flag. Before the collapse, the categorize_transaction tool
// was the canonical override-setter; folding it into update_transactions
// made it easy to drop the override flag silently. This test would catch
// that regression.
func TestApplyRulesRespectsOverrideFromUpdateTransactions(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// Two categories: the user-pinned one and the one the rule would set.
	pinnedCat := testutil.MustCreateCategory(t, queries, "personal_misc", "Personal")
	testutil.MustCreateCategory(t, queries, "food_and_drink_groceries", "Groceries")

	// Transaction that would otherwise match a "Costco → groceries" rule.
	txn := testutil.MustCreateTransaction(t, queries, acctID, "extOverride", "Costco Wholesale", 5000, "2026-04-06")

	// Pin via update_transactions (the canonical override-setter post-collapse).
	if _, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{{
			TransactionID: txn.ShortID,
			CategorySlug:  strPtr("personal_misc"),
		}},
		Actor: service.Actor{Type: "user", ID: "u1", Name: "Tester"},
	}); err != nil {
		t.Fatalf("seed override via update_transactions: %v", err)
	}

	// Create a competing rule that would set groceries.
	if _, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name: "Costco → Groceries",
		Conditions: service.Condition{
			Field: "provider_name",
			Op:    "contains",
			Value: "Costco",
		},
		CategorySlug: "food_and_drink_groceries",
		Priority:     100,
		Actor:        service.Actor{Type: "system", Name: "test"},
	}); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	// Run the retroactive pipeline. The override must hold.
	if _, err := svc.ApplyAllRulesRetroactively(ctx); err != nil {
		t.Fatalf("ApplyAllRulesRetroactively: %v", err)
	}

	var catID pgtype.UUID
	if err := pool.QueryRow(ctx,
		"SELECT category_id FROM transactions WHERE id = $1", txn.ID).Scan(&catID); err != nil {
		t.Fatalf("query category_id: %v", err)
	}
	if catID != pinnedCat.ID {
		t.Errorf("expected user-pinned category to survive apply_rules; got category_id=%v want %v",
			catID, pinnedCat.ID)
	}
}
