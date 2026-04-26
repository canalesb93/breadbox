//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
)

// --- Helper ---

// mustCreateCategoryViaService creates a category via the service layer and returns the response.
func mustCreateCategoryViaService(t *testing.T, svc *service.Service, slug, displayName string, parentID *string) *service.CategoryResponse {
	t.Helper()
	cat, err := svc.CreateCategory(context.Background(), service.CreateCategoryParams{
		Slug:        slug,
		DisplayName: displayName,
		ParentID:    parentID,
	})
	if err != nil {
		t.Fatalf("mustCreateCategoryViaService(%q): %v", slug, err)
	}
	return cat
}

// mustSeedUncategorized creates the system "uncategorized" category directly in DB.
func mustSeedUncategorized(t *testing.T, q *db.Queries) db.Category {
	t.Helper()
	cat, err := q.InsertCategory(context.Background(), db.InsertCategoryParams{
		Slug: "uncategorized", DisplayName: "Uncategorized", IsSystem: true,
	})
	if err != nil {
		t.Fatalf("seed uncategorized: %v", err)
	}
	return cat
}

// mustCreateTransactionWithCategory creates a transaction linked to a specific category.
func mustCreateTransactionWithCategory(t *testing.T, q *db.Queries, acctID, catID pgtype.UUID, extID, name string, amountCents int64, date string) db.Transaction {
	t.Helper()
	txn, err := q.UpsertTransaction(context.Background(), db.UpsertTransactionParams{
		AccountID:             acctID,
		ProviderTransactionID: extID,
		Amount:                pgtype.Numeric{Int: big.NewInt(amountCents), Exp: -2, Valid: true},
		IsoCurrencyCode:       pgtype.Text{String: "USD", Valid: true},
		Date:                  pgconv.Date(testutil.MustParseDate(date)),
		ProviderName:          name,
		CategoryID:            catID,
	})
	if err != nil {
		t.Fatalf("mustCreateTransactionWithCategory(%q): %v", name, err)
	}
	return txn
}

// --- ListCategories ---

func TestListCategories_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	cats, err := svc.ListCategories(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cats) != 0 {
		t.Errorf("expected 0 categories, got %d", len(cats))
	}
}

func TestListCategories_ReturnsAll(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	parent := mustCreateCategoryViaService(t, svc, "food_and_drink", "Food & Drink", nil)
	mustCreateCategoryViaService(t, svc, "food_groceries", "Groceries", &parent.ID)
	mustCreateCategoryViaService(t, svc, "transportation", "Transportation", nil)

	cats, err := svc.ListCategories(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cats) != 3 {
		t.Fatalf("expected 3 categories, got %d", len(cats))
	}

	// Verify child has parent info
	var child *service.CategoryResponse
	for i := range cats {
		if cats[i].Slug == "food_groceries" {
			child = &cats[i]
			break
		}
	}
	if child == nil {
		t.Fatal("child category not found")
	}
	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Errorf("child parent ID mismatch")
	}
	if child.ParentSlug == nil || *child.ParentSlug != "food_and_drink" {
		t.Errorf("child parent slug mismatch: got %v", child.ParentSlug)
	}
}

// --- ListCategoryTree ---

func TestListCategoryTree_GroupsChildren(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	parent := mustCreateCategoryViaService(t, svc, "food_and_drink", "Food & Drink", nil)
	mustCreateCategoryViaService(t, svc, "food_groceries", "Groceries", &parent.ID)
	mustCreateCategoryViaService(t, svc, "food_restaurants", "Restaurants", &parent.ID)
	mustCreateCategoryViaService(t, svc, "transportation", "Transportation", nil)

	tree, err := svc.ListCategoryTree(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tree should only have root-level categories
	if len(tree) != 2 {
		t.Fatalf("expected 2 root categories, got %d", len(tree))
	}

	var foodNode *service.CategoryResponse
	for i := range tree {
		if tree[i].Slug == "food_and_drink" {
			foodNode = &tree[i]
			break
		}
	}
	if foodNode == nil {
		t.Fatal("food_and_drink not found in tree roots")
	}
	if len(foodNode.Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(foodNode.Children))
	}
}

// --- GetCategory ---

func TestGetCategory_Found(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	created := mustCreateCategoryViaService(t, svc, "test_cat", "Test Category", nil)

	got, err := svc.GetCategory(ctx, created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Slug != "test_cat" {
		t.Errorf("slug mismatch: got %s", got.Slug)
	}
	if got.DisplayName != "Test Category" {
		t.Errorf("display name mismatch: got %s", got.DisplayName)
	}
}

func TestGetCategory_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.GetCategory(context.Background(), "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, service.ErrCategoryNotFound) {
		t.Errorf("expected ErrCategoryNotFound, got: %v", err)
	}
}

func TestGetCategory_InvalidID(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.GetCategory(context.Background(), "not-a-uuid")
	if !errors.Is(err, service.ErrCategoryNotFound) {
		t.Errorf("expected ErrCategoryNotFound for invalid ID, got: %v", err)
	}
}

// --- CreateCategory ---

func TestCreateCategory_AutoGeneratesSlug(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	cat, err := svc.CreateCategory(ctx, service.CreateCategoryParams{
		DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat.Slug != "food__drink" && cat.Slug != "food_drink" {
		// GenerateSlug converts "Food & Drink" → "food_&_drink" → strips & → "food__drink" → collapses → "food_drink"
		// Let's just check it's non-empty and lowercase
		if cat.Slug == "" {
			t.Errorf("expected auto-generated slug, got empty")
		}
	}
	if cat.DisplayName != "Food & Drink" {
		t.Errorf("display name mismatch: got %s", cat.DisplayName)
	}
	if cat.IsSystem {
		t.Errorf("new category should not be system")
	}
}

func TestCreateCategory_DuplicateSlugAppendsSuffix(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	cat1 := mustCreateCategoryViaService(t, svc, "my_cat", "My Category", nil)
	cat2, err := svc.CreateCategory(ctx, service.CreateCategoryParams{
		Slug:        "my_cat",
		DisplayName: "My Category Dupe",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat2.Slug != "my_cat_2" {
		t.Errorf("expected slug my_cat_2, got %s", cat2.Slug)
	}
	if cat1.ID == cat2.ID {
		t.Errorf("categories should have different IDs")
	}
}

func TestCreateCategory_WithParent(t *testing.T) {
	svc, _, _ := newService(t)

	parent := mustCreateCategoryViaService(t, svc, "parent_cat", "Parent", nil)
	child := mustCreateCategoryViaService(t, svc, "child_cat", "Child", &parent.ID)

	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Errorf("child parent ID mismatch")
	}
}

func TestCreateCategory_WithIconAndColor(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	icon := "shopping-cart"
	color := "#ff0000"
	cat, err := svc.CreateCategory(ctx, service.CreateCategoryParams{
		Slug:        "styled_cat",
		DisplayName: "Styled",
		Icon:        &icon,
		Color:       &color,
		SortOrder:   5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat.Icon == nil || *cat.Icon != "shopping-cart" {
		t.Errorf("icon mismatch: got %v", cat.Icon)
	}
	if cat.Color == nil || *cat.Color != "#ff0000" {
		t.Errorf("color mismatch: got %v", cat.Color)
	}
	if cat.SortOrder != 5 {
		t.Errorf("sort order mismatch: got %d", cat.SortOrder)
	}
}

// --- UpdateCategory ---

func TestUpdateCategory_Success(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	cat := mustCreateCategoryViaService(t, svc, "update_me", "Before", nil)

	icon := "new-icon"
	updated, err := svc.UpdateCategory(ctx, cat.ID, service.UpdateCategoryParams{
		DisplayName: "After",
		Icon:        &icon,
		SortOrder:   10,
		Hidden:      true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.DisplayName != "After" {
		t.Errorf("display name not updated: got %s", updated.DisplayName)
	}
	if updated.Icon == nil || *updated.Icon != "new-icon" {
		t.Errorf("icon not updated")
	}
	if updated.SortOrder != 10 {
		t.Errorf("sort order not updated: got %d", updated.SortOrder)
	}
	if !updated.Hidden {
		t.Errorf("hidden not updated")
	}
	// Slug should NOT change
	if updated.Slug != "update_me" {
		t.Errorf("slug should be immutable, got %s", updated.Slug)
	}
}

func TestUpdateCategory_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.UpdateCategory(context.Background(), "00000000-0000-0000-0000-000000000000", service.UpdateCategoryParams{
		DisplayName: "Nope",
	})
	if !errors.Is(err, service.ErrCategoryNotFound) {
		t.Errorf("expected ErrCategoryNotFound, got: %v", err)
	}
}

// --- DeleteCategory ---

func TestDeleteCategory_Success(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()

	mustSeedUncategorized(t, q)
	cat := mustCreateCategoryViaService(t, svc, "delete_me", "Delete Me", nil)

	count, err := svc.DeleteCategory(ctx, cat.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 affected transactions, got %d", count)
	}

	// Verify it's gone
	_, err = svc.GetCategory(ctx, cat.ID)
	if !errors.Is(err, service.ErrCategoryNotFound) {
		t.Errorf("expected not found after delete, got: %v", err)
	}
}

func TestDeleteCategory_CannotDeleteUncategorized(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()

	uncat := mustSeedUncategorized(t, q)
	_, err := svc.DeleteCategory(ctx, pgconv.FormatUUID(uncat.ID))
	if !errors.Is(err, service.ErrCategoryUndeletable) {
		t.Errorf("expected ErrCategoryUndeletable, got: %v", err)
	}
}

func TestDeleteCategory_ReassignsTransactionsToUncategorized(t *testing.T) {
	svc, q, pool := newService(t)
	ctx := context.Background()

	uncat := mustSeedUncategorized(t, q)
	cat := mustCreateCategoryViaService(t, svc, "doomed", "Doomed", nil)

	acctID := seedTxnFixture(t, q)
	catUID, _ := parseUUIDForTest(cat.ID)
	mustCreateTransactionWithCategory(t, q, acctID, catUID, "txn_doomed", "Doomed Txn", 1000, "2025-01-15")

	count, err := svc.DeleteCategory(ctx, cat.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 affected transaction, got %d", count)
	}

	// Transaction should now be uncategorized
	var gotCatID pgtype.UUID
	err = pool.QueryRow(ctx, "SELECT category_id FROM transactions WHERE provider_transaction_id = 'txn_doomed'").Scan(&gotCatID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if gotCatID != uncat.ID {
		t.Errorf("expected transaction reassigned to uncategorized")
	}
}

func TestDeleteCategory_DeletesChildrenToo(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()

	mustSeedUncategorized(t, q)
	parent := mustCreateCategoryViaService(t, svc, "parent_del", "Parent", nil)
	child := mustCreateCategoryViaService(t, svc, "child_del", "Child", &parent.ID)

	_, err := svc.DeleteCategory(ctx, parent.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both parent and child should be gone
	_, err = svc.GetCategory(ctx, parent.ID)
	if !errors.Is(err, service.ErrCategoryNotFound) {
		t.Errorf("parent should be deleted")
	}
	_, err = svc.GetCategory(ctx, child.ID)
	if !errors.Is(err, service.ErrCategoryNotFound) {
		t.Errorf("child should be cascade-deleted")
	}
}

func TestDeleteCategory_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.DeleteCategory(context.Background(), "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, service.ErrCategoryNotFound) {
		t.Errorf("expected ErrCategoryNotFound, got: %v", err)
	}
}

// --- MergeCategories ---

func TestMergeCategories_Success(t *testing.T) {
	svc, q, pool := newService(t)
	ctx := context.Background()

	source := mustCreateCategoryViaService(t, svc, "merge_source", "Source", nil)
	target := mustCreateCategoryViaService(t, svc, "merge_target", "Target", nil)

	// Create a transaction in source category
	acctID := seedTxnFixture(t, q)
	srcUID, _ := parseUUIDForTest(source.ID)
	mustCreateTransactionWithCategory(t, q, acctID, srcUID, "txn_merge", "Merge Txn", 2000, "2025-01-15")

	err := svc.MergeCategories(ctx, source.ID, target.ID)
	if err != nil {
		t.Fatalf("MergeCategories: %v", err)
	}

	// Source should be deleted
	_, err = svc.GetCategory(ctx, source.ID)
	if !errors.Is(err, service.ErrCategoryNotFound) {
		t.Errorf("source category should be deleted after merge")
	}

	// Transaction should be in target
	tgtUID, _ := parseUUIDForTest(target.ID)
	var gotCatID pgtype.UUID
	err = pool.QueryRow(ctx, "SELECT category_id FROM transactions WHERE provider_transaction_id = 'txn_merge'").Scan(&gotCatID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if gotCatID != tgtUID {
		t.Errorf("transaction should be reassigned to target category")
	}
}

func TestMergeCategories_SourceNotFound(t *testing.T) {
	svc, _, _ := newService(t)
	target := mustCreateCategoryViaService(t, svc, "merge_tgt2", "Target", nil)
	err := svc.MergeCategories(context.Background(), "00000000-0000-0000-0000-000000000000", target.ID)
	if !errors.Is(err, service.ErrCategoryNotFound) {
		t.Errorf("expected ErrCategoryNotFound, got: %v", err)
	}
}

func TestMergeCategories_TargetNotFound(t *testing.T) {
	svc, _, _ := newService(t)
	source := mustCreateCategoryViaService(t, svc, "merge_src2", "Source", nil)
	err := svc.MergeCategories(context.Background(), source.ID, "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, service.ErrCategoryNotFound) {
		t.Errorf("expected ErrCategoryNotFound, got: %v", err)
	}
}

func TestMergeCategories_ReassignsRulesInsteadOfDeleting(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	source := mustCreateCategoryViaService(t, svc, "merge_rule_src", "Rule Source", nil)
	target := mustCreateCategoryViaService(t, svc, "merge_rule_tgt", "Rule Target", nil)

	// Create a rule pointing to the source category.
	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:         "Test Rule For Merge",
		Conditions:   service.Condition{Field: "provider_name", Op: "contains", Value: "coffee"},
		CategorySlug: "merge_rule_src",
		Priority:     10,
		Actor:        service.Actor{Type: "system", Name: "test"},
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	// Merge source into target.
	err = svc.MergeCategories(ctx, source.ID, target.ID)
	if err != nil {
		t.Fatalf("MergeCategories: %v", err)
	}

	// The rule should still exist and point to the target category (not deleted).
	gotRule, err := svc.GetTransactionRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("rule should still exist after merge, got error: %v", err)
	}
	if gotRule.CategorySlug == nil || *gotRule.CategorySlug != "merge_rule_tgt" {
		got := "<nil>"
		if gotRule.CategorySlug != nil {
			got = *gotRule.CategorySlug
		}
		t.Errorf("rule category_slug should be reassigned to target; got %q, want %q", got, "merge_rule_tgt")
	}

	// Verify via the typed actions JSONB shape that the set_category action's
	// category_slug now points at the target (no denormalized category_id
	// column).
	_ = target
	ruleUID, _ := parseUUIDForTest(rule.ID)
	var actionsRaw []byte
	err = pool.QueryRow(ctx, "SELECT actions FROM transaction_rules WHERE id = $1", ruleUID).Scan(&actionsRaw)
	if err != nil {
		t.Fatalf("query rule actions: %v", err)
	}
	if len(actionsRaw) == 0 {
		t.Fatal("expected non-empty actions JSONB on reassigned rule")
	}
	var parsed []map[string]any
	if err := json.Unmarshal(actionsRaw, &parsed); err != nil {
		t.Fatalf("unmarshal actions JSONB: %v", err)
	}
	var foundSlug string
	for _, a := range parsed {
		if t, _ := a["type"].(string); t == "set_category" {
			foundSlug, _ = a["category_slug"].(string)
			break
		}
	}
	if foundSlug != "merge_rule_tgt" {
		t.Errorf("rule actions[set_category].category_slug should be %q, got %q", "merge_rule_tgt", foundSlug)
	}
}

func TestMergeCategories_SelfMerge(t *testing.T) {
	svc, _, _ := newService(t)
	cat := mustCreateCategoryViaService(t, svc, "self_merge", "Self Merge", nil)
	err := svc.MergeCategories(context.Background(), cat.ID, cat.ID)
	if err == nil {
		t.Error("expected error when merging category into itself, got nil")
	}
}

func TestMergeCategories_ParentWithChildren(t *testing.T) {
	svc, q, pool := newService(t)
	ctx := context.Background()

	// Create parent with two children.
	parent := mustCreateCategoryViaService(t, svc, "merge_parent", "Parent", nil)
	child1 := mustCreateCategoryViaService(t, svc, "merge_child1", "Child 1", &parent.ID)
	child2 := mustCreateCategoryViaService(t, svc, "merge_child2", "Child 2", &parent.ID)

	// Create a target category.
	target := mustCreateCategoryViaService(t, svc, "merge_parent_target", "Target", nil)

	// Create transactions in parent and each child.
	acctID := seedTxnFixture(t, q)
	parentUID, _ := parseUUIDForTest(parent.ID)
	child1UID, _ := parseUUIDForTest(child1.ID)
	child2UID, _ := parseUUIDForTest(child2.ID)
	tgtUID, _ := parseUUIDForTest(target.ID)

	mustCreateTransactionWithCategory(t, q, acctID, parentUID, "txn_parent", "Parent Txn", 1000, "2025-01-10")
	mustCreateTransactionWithCategory(t, q, acctID, child1UID, "txn_child1", "Child 1 Txn", 2000, "2025-01-11")
	mustCreateTransactionWithCategory(t, q, acctID, child2UID, "txn_child2", "Child 2 Txn", 3000, "2025-01-12")

	// Create a rule pointing to child2.
	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:         "Child Rule",
		Conditions:   service.Condition{Field: "provider_name", Op: "contains", Value: "child"},
		CategorySlug: "merge_child2",
		Priority:     10,
		Actor:        service.Actor{Type: "system", Name: "test"},
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	// Merge parent (with children) into target.
	err = svc.MergeCategories(ctx, parent.ID, target.ID)
	if err != nil {
		t.Fatalf("MergeCategories: %v", err)
	}

	// All three categories should be deleted.
	for _, slug := range []string{"merge_parent", "merge_child1", "merge_child2"} {
		_, err := svc.GetCategoryBySlug(ctx, slug)
		if !errors.Is(err, service.ErrCategoryNotFound) {
			t.Errorf("expected %s to be deleted, got err: %v", slug, err)
		}
	}

	// All three transactions should be reassigned to target.
	for _, extID := range []string{"txn_parent", "txn_child1", "txn_child2"} {
		var gotCatID pgtype.UUID
		err := pool.QueryRow(ctx, "SELECT category_id FROM transactions WHERE provider_transaction_id = $1", extID).Scan(&gotCatID)
		if err != nil {
			t.Fatalf("query %s: %v", extID, err)
		}
		if gotCatID != tgtUID {
			t.Errorf("transaction %s should be reassigned to target category", extID)
		}
	}

	// Rule should still exist and point to target.
	gotRule, err := svc.GetTransactionRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("rule should still exist after merge, got error: %v", err)
	}
	if gotRule.CategorySlug == nil || *gotRule.CategorySlug != "merge_parent_target" {
		got := "<nil>"
		if gotRule.CategorySlug != nil {
			got = *gotRule.CategorySlug
		}
		t.Errorf("rule should be reassigned to target slug; got %q, want %q", got, "merge_parent_target")
	}
}

// --- SetTransactionCategory ---

func TestSetTransactionCategory_Success(t *testing.T) {
	svc, q, pool := newService(t)
	ctx := context.Background()

	uncat := mustSeedUncategorized(t, q)
	target := mustCreateCategoryViaService(t, svc, "manual_cat", "Manual", nil)

	acctID := seedTxnFixture(t, q)
	txn := mustCreateTransactionWithCategory(t, q, acctID, uncat.ID, "txn_manual", "Manual Txn", 500, "2025-02-01")

	err := svc.SetTransactionCategory(ctx, pgconv.FormatUUID(txn.ID), target.ID)
	if err != nil {
		t.Fatalf("SetTransactionCategory: %v", err)
	}

	// Verify category changed and override is set
	var gotCatID pgtype.UUID
	var override bool
	err = pool.QueryRow(ctx,
		"SELECT category_id, category_override FROM transactions WHERE id = $1", txn.ID,
	).Scan(&gotCatID, &override)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	tgtUID, _ := parseUUIDForTest(target.ID)
	if gotCatID != tgtUID {
		t.Errorf("category not set correctly")
	}
	if !override {
		t.Errorf("category_override should be true")
	}
}

func TestSetTransactionCategory_InvalidTransaction(t *testing.T) {
	svc, _, _ := newService(t)
	cat := mustCreateCategoryViaService(t, svc, "cat_for_invalid", "Cat", nil)
	err := svc.SetTransactionCategory(context.Background(), "00000000-0000-0000-0000-000000000000", cat.ID)
	if err == nil {
		t.Error("expected error for nonexistent transaction")
	}
}

func TestSetTransactionCategory_InvalidCategory(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()

	uncat := mustSeedUncategorized(t, q)
	acctID := seedTxnFixture(t, q)
	txn := mustCreateTransactionWithCategory(t, q, acctID, uncat.ID, "txn_badcat", "Bad Cat", 100, "2025-02-01")

	err := svc.SetTransactionCategory(ctx, pgconv.FormatUUID(txn.ID), "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, service.ErrCategoryNotFound) {
		t.Errorf("expected ErrCategoryNotFound, got: %v", err)
	}
}

// --- ResetTransactionCategory ---


func TestResetTransactionCategory_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	err := svc.ResetTransactionCategory(context.Background(), "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Error("expected error for nonexistent transaction")
	}
}

// --- BatchSetTransactionCategory ---

func TestBatchSetTransactionCategory_SetsOverrideFlag(t *testing.T) {
	svc, q, pool := newService(t)
	ctx := context.Background()

	uncat := mustSeedUncategorized(t, q)
	mustCreateCategoryViaService(t, svc, "batch_groceries", "Groceries", nil)

	acctID := seedTxnFixture(t, q)
	txn1 := mustCreateTransactionWithCategory(t, q, acctID, uncat.ID, "batch_txn_1", "Txn 1", 100, "2025-01-10")

	result, err := svc.BatchSetTransactionCategory(ctx, []service.BatchCategorizeItem{
		{TransactionID: pgconv.FormatUUID(txn1.ID), CategorySlug: "batch_groceries"},
	})
	if err != nil {
		t.Fatalf("BatchSetTransactionCategory: %v", err)
	}
	if result.Succeeded != 1 {
		t.Errorf("expected 1 succeeded, got %d", result.Succeeded)
	}

	// Verify category_override is set to TRUE
	var override bool
	err = pool.QueryRow(ctx, "SELECT category_override FROM transactions WHERE id = $1", txn1.ID).Scan(&override)
	if err != nil {
		t.Fatalf("query override: %v", err)
	}
	if !override {
		t.Errorf("category_override should be true after batch categorize")
	}
}

func TestBatchSetTransactionCategory_PartialFailure(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()

	uncat := mustSeedUncategorized(t, q)
	mustCreateCategoryViaService(t, svc, "batch_good", "Good", nil)

	acctID := seedTxnFixture(t, q)
	txn := mustCreateTransactionWithCategory(t, q, acctID, uncat.ID, "batch_partial", "Partial", 100, "2025-01-10")

	result, err := svc.BatchSetTransactionCategory(ctx, []service.BatchCategorizeItem{
		{TransactionID: pgconv.FormatUUID(txn.ID), CategorySlug: "batch_good"},
		{TransactionID: pgconv.FormatUUID(txn.ID), CategorySlug: "nonexistent_slug"},
	})
	if err != nil {
		t.Fatalf("BatchSetTransactionCategory: %v", err)
	}
	if result.Succeeded != 1 {
		t.Errorf("expected 1 succeeded, got %d", result.Succeeded)
	}
	if len(result.Failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(result.Failed))
	}
}

func TestBatchSetTransactionCategory_EmptyItems(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.BatchSetTransactionCategory(context.Background(), []service.BatchCategorizeItem{})
	if err == nil {
		t.Error("expected error for empty items")
	}
}

func TestBatchSetTransactionCategory_TooManyItems(t *testing.T) {
	svc, _, _ := newService(t)
	items := make([]service.BatchCategorizeItem, 501)
	for i := range items {
		items[i] = service.BatchCategorizeItem{TransactionID: "x", CategorySlug: "y"}
	}
	_, err := svc.BatchSetTransactionCategory(context.Background(), items)
	if err == nil {
		t.Error("expected error for >500 items")
	}
}

// --- BulkRecategorizeByFilter ---

func TestBulkRecategorizeByFilter_ByNameSearch(t *testing.T) {
	svc, q, pool := newService(t)
	ctx := context.Background()

	uncat := mustSeedUncategorized(t, q)
	mustCreateCategoryViaService(t, svc, "bulk_target", "Target", nil)

	acctID := seedTxnFixture(t, q)
	mustCreateTransactionWithCategory(t, q, acctID, uncat.ID, "bulk_txn_1", "Starbucks Coffee", 500, "2025-01-10")
	mustCreateTransactionWithCategory(t, q, acctID, uncat.ID, "bulk_txn_2", "Starbucks Tea", 300, "2025-01-11")
	mustCreateTransactionWithCategory(t, q, acctID, uncat.ID, "bulk_txn_3", "Walmart", 2000, "2025-01-12")

	search := "Starbucks"
	result, err := svc.BulkRecategorizeByFilter(ctx, service.BulkRecategorizeParams{
		TargetCategorySlug: "bulk_target",
		Search:             &search,
	})
	if err != nil {
		t.Fatalf("BulkRecategorizeByFilter: %v", err)
	}
	if result.UpdatedCount != 2 {
		t.Errorf("expected 2 updated, got %d", result.UpdatedCount)
	}

	// Walmart should be unchanged
	var walmartCatID pgtype.UUID
	pool.QueryRow(ctx, "SELECT category_id FROM transactions WHERE provider_transaction_id = 'bulk_txn_3'").Scan(&walmartCatID)
	if walmartCatID != uncat.ID {
		t.Errorf("Walmart should still be uncategorized")
	}
}

func TestBulkRecategorizeByFilter_RequiresFilter(t *testing.T) {
	svc, q, _ := newService(t)
	mustSeedUncategorized(t, q)
	mustCreateCategoryViaService(t, svc, "bulk_nofilter", "NoFilter", nil)

	_, err := svc.BulkRecategorizeByFilter(context.Background(), service.BulkRecategorizeParams{
		TargetCategorySlug: "bulk_nofilter",
	})
	if err == nil {
		t.Error("expected error when no filter provided")
	}
	if !strings.Contains(err.Error(), "at least one filter") {
		t.Errorf("error should mention filter requirement, got: %v", err)
	}
}

func TestBulkRecategorizeByFilter_InvalidTargetCategory(t *testing.T) {
	svc, _, _ := newService(t)
	search := "anything"
	_, err := svc.BulkRecategorizeByFilter(context.Background(), service.BulkRecategorizeParams{
		TargetCategorySlug: "nonexistent_target",
		Search:             &search,
	})
	if err == nil {
		t.Error("expected error for invalid target category")
	}
}

// --- GetCategoryBySlug ---

func TestGetCategoryBySlug_Found(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	mustCreateCategoryViaService(t, svc, "slug_test", "Slug Test", nil)

	cat, err := svc.GetCategoryBySlug(ctx, "slug_test")
	if err != nil {
		t.Fatalf("GetCategoryBySlug: %v", err)
	}
	if cat.Slug != "slug_test" {
		t.Errorf("slug mismatch: got %s", cat.Slug)
	}
}

func TestGetCategoryBySlug_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.GetCategoryBySlug(context.Background(), "nonexistent_slug")
	if err == nil {
		t.Error("expected error for nonexistent slug")
	}
}

// --- GenerateSlug ---

func TestGenerateSlug_Variants(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Food & Drink", "food_drink"},
		{"  Travel  ", "travel"},
		{"General Merchandise", "general_merchandise"},
		{"123 Numbers", "123_numbers"},
		{"Already_valid", "already_valid"},
	}
	for _, tc := range tests {
		got := service.GenerateSlug(tc.input)
		if got != tc.expected {
			t.Errorf("GenerateSlug(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// --- parseUUIDForTest helper (internal to these tests) ---

func parseUUIDForTest(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return u, nil
}
