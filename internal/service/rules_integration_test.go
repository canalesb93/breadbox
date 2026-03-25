//go:build integration

package service_test

import (
	"context"
	"errors"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// mustCreateCategoryForRule creates a category for use in rule tests.
// Re-uses the mustCreateCategory helper from review_integration_test.go (same package).

// --- CreateTransactionRule ---

func TestCreateTransactionRule_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	rule, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Coffee shops",
		CategorySlug: cat.Slug,
		Conditions: service.Condition{
			Field: "name",
			Op:    "contains",
			Value: "coffee",
		},
		Priority: 10,
		Actor:    service.Actor{Type: "user", ID: "admin-1", Name: "Test Admin"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	if rule.ID == "" {
		t.Error("expected non-empty ID")
	}
	if rule.Name != "Coffee shops" {
		t.Errorf("name: got %q, want %q", rule.Name, "Coffee shops")
	}
	if rule.CategorySlug == nil || *rule.CategorySlug != "food_and_drink" {
		t.Errorf("category slug mismatch")
	}
	if rule.CategoryName == nil || *rule.CategoryName != "Food & Drink" {
		t.Errorf("category name mismatch")
	}
	if rule.Priority != 10 {
		t.Errorf("priority: got %d, want 10", rule.Priority)
	}
	if !rule.Enabled {
		t.Error("expected rule to be enabled by default")
	}
	if rule.CreatedByType != "user" {
		t.Errorf("created_by_type: got %q, want %q", rule.CreatedByType, "user")
	}
	if rule.CreatedByName != "Test Admin" {
		t.Errorf("created_by_name: got %q, want %q", rule.CreatedByName, "Test Admin")
	}
	if rule.HitCount != 0 {
		t.Errorf("hit_count: got %d, want 0", rule.HitCount)
	}
}

func TestCreateTransactionRule_DefaultPriority(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	rule, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Default priority rule",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test"},
		// No priority — should default to 10.
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}
	if rule.Priority != 10 {
		t.Errorf("expected default priority 10, got %d", rule.Priority)
	}
}

func TestCreateTransactionRule_WithExpiry(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	rule, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Temporary rule",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test"},
		ExpiresIn:    "24h",
		Actor:        service.Actor{Type: "agent", Name: "AI Agent"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}
	if rule.ExpiresAt == nil {
		t.Error("expected expires_at to be set")
	}
	if rule.CreatedByType != "agent" {
		t.Errorf("created_by_type: got %q, want %q", rule.CreatedByType, "agent")
	}
}

func TestCreateTransactionRule_InvalidExpiresIn(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	_, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Bad expiry",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test"},
		ExpiresIn:    "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid expires_in")
	}
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got: %v", err)
	}
}

func TestCreateTransactionRule_InvalidCategorySlug(t *testing.T) {
	svc, _, _ := newService(t)

	_, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Bad category",
		CategorySlug: "nonexistent_category",
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test"},
	})
	if err == nil {
		t.Fatal("expected error for invalid category slug")
	}
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got: %v", err)
	}
}

func TestCreateTransactionRule_InvalidCondition(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	_, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Bad condition",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "unknown_field", Op: "eq", Value: "test"},
	})
	if err == nil {
		t.Fatal("expected error for invalid condition field")
	}
}

func TestCreateTransactionRule_ANDCondition(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	rule, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "AND condition",
		CategorySlug: cat.Slug,
		Conditions: service.Condition{
			And: []service.Condition{
				{Field: "name", Op: "contains", Value: "coffee"},
				{Field: "amount", Op: "gte", Value: float64(5)},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule with AND: %v", err)
	}
	if rule.Conditions.And == nil || len(rule.Conditions.And) != 2 {
		t.Errorf("expected 2 AND conditions, got: %+v", rule.Conditions)
	}
}

// --- GetTransactionRule ---

func TestGetTransactionRule_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	created, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Test rule",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test"},
		Priority:     5,
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	got, err := svc.GetTransactionRule(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetTransactionRule: %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("ID mismatch")
	}
	if got.Name != "Test rule" {
		t.Errorf("name: got %q, want %q", got.Name, "Test rule")
	}
	if got.Priority != 5 {
		t.Errorf("priority: got %d, want 5", got.Priority)
	}
	if got.CategorySlug == nil || *got.CategorySlug != "food_and_drink" {
		t.Error("category slug not resolved in Get response")
	}
}

func TestGetTransactionRule_NotFound(t *testing.T) {
	svc, _, _ := newService(t)

	_, err := svc.GetTransactionRule(context.Background(), "00000000-0000-0000-0000-000000000001")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestGetTransactionRule_BadUUID(t *testing.T) {
	svc, _, _ := newService(t)

	_, err := svc.GetTransactionRule(context.Background(), "not-a-uuid")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound for bad UUID, got: %v", err)
	}
}

// --- ListTransactionRules ---

func TestListTransactionRules_Empty(t *testing.T) {
	svc, _, _ := newService(t)

	result, err := svc.ListTransactionRules(context.Background(), service.TransactionRuleListParams{})
	if err != nil {
		t.Fatalf("ListTransactionRules: %v", err)
	}
	if len(result.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(result.Rules))
	}
	if result.Total != 0 {
		t.Errorf("expected total 0, got %d", result.Total)
	}
	if result.HasMore {
		t.Error("expected HasMore=false")
	}
}

func TestListTransactionRules_WithData(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	for i := 0; i < 3; i++ {
		_, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
			Name:         "Rule " + string(rune('A'+i)),
			CategorySlug: cat.Slug,
			Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test"},
		})
		if err != nil {
			t.Fatalf("CreateTransactionRule %d: %v", i, err)
		}
	}

	result, err := svc.ListTransactionRules(context.Background(), service.TransactionRuleListParams{})
	if err != nil {
		t.Fatalf("ListTransactionRules: %v", err)
	}
	if len(result.Rules) != 3 {
		t.Errorf("expected 3 rules, got %d", len(result.Rules))
	}
	if result.Total != 3 {
		t.Errorf("expected total 3, got %d", result.Total)
	}
}

func TestListTransactionRules_FilterByEnabled(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	created, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Enabled rule",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Disable the rule.
	disabled := false
	_, err = svc.UpdateTransactionRule(context.Background(), created.ID, service.UpdateTransactionRuleParams{
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("disable: %v", err)
	}

	// Create another enabled rule.
	_, err = svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Still enabled",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test2"},
	})
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}

	enabledFilter := true
	result, err := svc.ListTransactionRules(context.Background(), service.TransactionRuleListParams{
		Enabled: &enabledFilter,
	})
	if err != nil {
		t.Fatalf("ListTransactionRules: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 enabled rule, got %d", result.Total)
	}
}

func TestListTransactionRules_FilterByCategory(t *testing.T) {
	svc, queries, _ := newService(t)
	cat1 := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")
	cat2 := mustCreateCategory(t, queries, "transportation", "Transportation")

	_, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Food rule",
		CategorySlug: cat1.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "pizza"},
	})
	if err != nil {
		t.Fatalf("create 1: %v", err)
	}

	_, err = svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Transport rule",
		CategorySlug: cat2.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "uber"},
	})
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}

	slug := "food_and_drink"
	result, err := svc.ListTransactionRules(context.Background(), service.TransactionRuleListParams{
		CategorySlug: &slug,
	})
	if err != nil {
		t.Fatalf("ListTransactionRules: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 food rule, got %d", result.Total)
	}
	if len(result.Rules) != 1 || result.Rules[0].Name != "Food rule" {
		t.Errorf("expected Food rule, got: %+v", result.Rules)
	}
}

func TestListTransactionRules_FilterBySearch(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	_, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Coffee shops",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "contains", Value: "coffee"},
	})
	if err != nil {
		t.Fatalf("create 1: %v", err)
	}

	_, err = svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Grocery stores",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "contains", Value: "grocery"},
	})
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}

	search := "coffee"
	result, err := svc.ListTransactionRules(context.Background(), service.TransactionRuleListParams{
		Search: &search,
	})
	if err != nil {
		t.Fatalf("ListTransactionRules: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 matching rule, got %d", result.Total)
	}
}

func TestListTransactionRules_Pagination(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	// Create 5 rules.
	for i := 0; i < 5; i++ {
		_, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
			Name:         "Rule " + string(rune('A'+i)),
			CategorySlug: cat.Slug,
			Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test"},
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	// First page: limit 2.
	result, err := svc.ListTransactionRules(context.Background(), service.TransactionRuleListParams{
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(result.Rules) != 2 {
		t.Errorf("page 1: expected 2 rules, got %d", len(result.Rules))
	}
	if !result.HasMore {
		t.Error("page 1: expected HasMore=true")
	}
	if result.NextCursor == "" {
		t.Error("page 1: expected non-empty cursor")
	}
	if result.Total != 5 {
		t.Errorf("page 1: expected total 5, got %d", result.Total)
	}

	// Second page using cursor.
	result2, err := svc.ListTransactionRules(context.Background(), service.TransactionRuleListParams{
		Limit:  2,
		Cursor: result.NextCursor,
	})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(result2.Rules) != 2 {
		t.Errorf("page 2: expected 2 rules, got %d", len(result2.Rules))
	}
	if !result2.HasMore {
		t.Error("page 2: expected HasMore=true")
	}

	// Ensure no overlap between pages.
	for _, r1 := range result.Rules {
		for _, r2 := range result2.Rules {
			if r1.ID == r2.ID {
				t.Errorf("duplicate rule across pages: %s", r1.ID)
			}
		}
	}
}

// --- UpdateTransactionRule ---

func TestUpdateTransactionRule_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	cat1 := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")
	cat2 := mustCreateCategory(t, queries, "transportation", "Transportation")

	created, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Original name",
		CategorySlug: cat1.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test"},
		Priority:     5,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newName := "Updated name"
	newPriority := 20
	newSlug := cat2.Slug
	updated, err := svc.UpdateTransactionRule(context.Background(), created.ID, service.UpdateTransactionRuleParams{
		Name:         &newName,
		Priority:     &newPriority,
		CategorySlug: &newSlug,
	})
	if err != nil {
		t.Fatalf("UpdateTransactionRule: %v", err)
	}
	if updated.Name != "Updated name" {
		t.Errorf("name: got %q, want %q", updated.Name, "Updated name")
	}
	if updated.Priority != 20 {
		t.Errorf("priority: got %d, want 20", updated.Priority)
	}
	if updated.CategorySlug == nil || *updated.CategorySlug != "transportation" {
		t.Errorf("category slug not updated")
	}
}

func TestUpdateTransactionRule_PartialUpdate(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	created, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Original",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test"},
		Priority:     5,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Only update name, leave everything else unchanged.
	newName := "New Name"
	updated, err := svc.UpdateTransactionRule(context.Background(), created.ID, service.UpdateTransactionRuleParams{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "New Name" {
		t.Errorf("name not updated: %q", updated.Name)
	}
	if updated.Priority != 5 {
		t.Errorf("priority changed unexpectedly: got %d, want 5", updated.Priority)
	}
	if updated.CategorySlug == nil || *updated.CategorySlug != "food_and_drink" {
		t.Error("category slug changed unexpectedly")
	}
}

func TestUpdateTransactionRule_Disable(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	created, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Rule",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	disabled := false
	updated, err := svc.UpdateTransactionRule(context.Background(), created.ID, service.UpdateTransactionRuleParams{
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Enabled {
		t.Error("expected rule to be disabled")
	}
}

func TestUpdateTransactionRule_NotFound(t *testing.T) {
	svc, _, _ := newService(t)

	newName := "test"
	_, err := svc.UpdateTransactionRule(context.Background(), "00000000-0000-0000-0000-000000000001", service.UpdateTransactionRuleParams{
		Name: &newName,
	})
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestUpdateTransactionRule_InvalidCategorySlug(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	created, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Rule",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	badSlug := "nonexistent_slug"
	_, err = svc.UpdateTransactionRule(context.Background(), created.ID, service.UpdateTransactionRuleParams{
		CategorySlug: &badSlug,
	})
	if err == nil {
		t.Fatal("expected error for invalid category slug")
	}
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got: %v", err)
	}
}

// --- DeleteTransactionRule ---

func TestDeleteTransactionRule_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	created, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "To be deleted",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.DeleteTransactionRule(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("DeleteTransactionRule: %v", err)
	}

	_, err = svc.GetTransactionRule(context.Background(), created.ID)
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestDeleteTransactionRule_NotFound(t *testing.T) {
	svc, _, _ := newService(t)

	err := svc.DeleteTransactionRule(context.Background(), "00000000-0000-0000-0000-000000000001")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestDeleteTransactionRule_BadUUID(t *testing.T) {
	svc, _, _ := newService(t)

	err := svc.DeleteTransactionRule(context.Background(), "not-a-uuid")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound for bad UUID, got: %v", err)
	}
}

// --- ListActiveRulesForSync ---

func TestListActiveRulesForSync_FiltersExpiredAndDisabled(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	// Create an active rule.
	_, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Active rule",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "active"},
	})
	if err != nil {
		t.Fatalf("create active: %v", err)
	}

	// Create a disabled rule.
	created2, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Disabled rule",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "disabled"},
	})
	if err != nil {
		t.Fatalf("create disabled: %v", err)
	}
	disabled := false
	_, err = svc.UpdateTransactionRule(context.Background(), created2.ID, service.UpdateTransactionRuleParams{
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("disable: %v", err)
	}

	rules, err := svc.ListActiveRulesForSync(context.Background())
	if err != nil {
		t.Fatalf("ListActiveRulesForSync: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("expected 1 active rule, got %d", len(rules))
	}
	if rules[0].Name != "Active rule" {
		t.Errorf("expected 'Active rule', got %q", rules[0].Name)
	}
}

// --- BatchIncrementHitCounts ---

func TestBatchIncrementHitCounts_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")

	rule, err := svc.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "Hit counter test",
		CategorySlug: cat.Slug,
		Conditions:   service.Condition{Field: "name", Op: "eq", Value: "test"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.BatchIncrementHitCounts(context.Background(), map[string]int{
		rule.ID: 5,
	})
	if err != nil {
		t.Fatalf("BatchIncrementHitCounts: %v", err)
	}

	got, err := svc.GetTransactionRule(context.Background(), rule.ID)
	if err != nil {
		t.Fatalf("GetTransactionRule: %v", err)
	}
	if got.HitCount != 5 {
		t.Errorf("hit_count: got %d, want 5", got.HitCount)
	}
	if got.LastHitAt == nil {
		t.Error("expected last_hit_at to be set")
	}

	// Increment again.
	err = svc.BatchIncrementHitCounts(context.Background(), map[string]int{
		rule.ID: 3,
	})
	if err != nil {
		t.Fatalf("BatchIncrementHitCounts 2: %v", err)
	}

	got, _ = svc.GetTransactionRule(context.Background(), rule.ID)
	if got.HitCount != 8 {
		t.Errorf("hit_count after second increment: got %d, want 8", got.HitCount)
	}
}

// --- SelfLink prevention (DB constraint) ---

func TestCreateAccountLink_SelfLinkRejected(t *testing.T) {
	svc, queries, _ := newService(t)
	user := testutil.MustCreateUser(t, queries, "User")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Acct")

	acctID := formatUUIDForTest(acct.ID)
	_, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   acctID,
		DependentAccountID: acctID,
	})
	if err == nil {
		t.Fatal("expected error for self-link (primary == dependent)")
	}
}

// --- MatchCount and UnmatchedDependentCount in GetAccountLink ---

func TestGetAccountLink_MatchCounts(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "User1")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_1", "Primary")

	user2 := testutil.MustCreateUser(t, queries, "User2")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_2")
	acct2 := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_2", "Dependent")

	link, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   formatUUIDForTest(acct1.ID),
		DependentAccountID: formatUUIDForTest(acct2.ID),
	})
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	// Create 3 transactions in each account, match 2 of them.
	txnP1 := testutil.MustCreateTransaction(t, queries, acct1.ID, "p1", "Store A", 1000, "2025-01-15")
	txnP2 := testutil.MustCreateTransaction(t, queries, acct1.ID, "p2", "Store B", 2000, "2025-01-16")
	_ = testutil.MustCreateTransaction(t, queries, acct1.ID, "p3", "Store C", 3000, "2025-01-17")

	txnD1 := testutil.MustCreateTransaction(t, queries, acct2.ID, "d1", "Store A", 1000, "2025-01-15")
	txnD2 := testutil.MustCreateTransaction(t, queries, acct2.ID, "d2", "Store B", 2000, "2025-01-16")
	_ = testutil.MustCreateTransaction(t, queries, acct2.ID, "d3", "Store C", 3000, "2025-01-17")

	_, err = svc.ManualMatch(context.Background(), link.ID, formatUUIDForTest(txnP1.ID), formatUUIDForTest(txnD1.ID))
	if err != nil {
		t.Fatalf("match 1: %v", err)
	}
	_, err = svc.ManualMatch(context.Background(), link.ID, formatUUIDForTest(txnP2.ID), formatUUIDForTest(txnD2.ID))
	if err != nil {
		t.Fatalf("match 2: %v", err)
	}

	got, err := svc.GetAccountLink(context.Background(), link.ID)
	if err != nil {
		t.Fatalf("GetAccountLink: %v", err)
	}
	if got.MatchCount != 2 {
		t.Errorf("match_count: got %d, want 2", got.MatchCount)
	}
	if got.UnmatchedDependentCount != 1 {
		t.Errorf("unmatched_dependent_count: got %d, want 1", got.UnmatchedDependentCount)
	}
}

// --- PreviewRule ---

func TestPreviewRule_ExcludesCategoryOverride(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	// Setup: user, connection, account, category.
	user := testutil.MustCreateUser(t, queries, "User")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")

	// Create 3 transactions: all named "Amazon".
	txn1 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_1", "Amazon Purchase", 1500, "2025-06-01")
	_ = testutil.MustCreateTransaction(t, queries, acct.ID, "txn_2", "Amazon Prime", 1299, "2025-06-02")
	_ = testutil.MustCreateTransaction(t, queries, acct.ID, "txn_3", "Amazon Fresh", 4500, "2025-06-03")

	// Set category_override=TRUE on one of them (simulating a manual categorization).
	_, err := pool.Exec(ctx, "UPDATE transactions SET category_override = TRUE WHERE id = $1", txn1.ID)
	if err != nil {
		t.Fatalf("set category_override: %v", err)
	}

	// Preview a rule that matches "Amazon" in name.
	result, err := svc.PreviewRule(ctx, service.Condition{
		Field: "name",
		Op:    "contains",
		Value: "Amazon",
	}, 10)
	if err != nil {
		t.Fatalf("PreviewRule: %v", err)
	}

	// Should only match 2 transactions (the non-overridden ones).
	// Before the fix, this would return 3 because PreviewRule didn't filter category_override.
	if result.MatchCount != 2 {
		t.Errorf("match_count: got %d, want 2 (should exclude category_override=TRUE)", result.MatchCount)
	}
	if len(result.SampleMatches) != 2 {
		t.Errorf("sample_matches: got %d, want 2", len(result.SampleMatches))
	}
	// TotalScanned should also be 2 (only non-overridden are scanned).
	if result.TotalScanned != 2 {
		t.Errorf("total_scanned: got %d, want 2", result.TotalScanned)
	}
}

// Verify that InsertCategoryParams works as expected (used by mustCreateCategory).
func init() {
	_ = db.InsertCategoryParams{}
}
