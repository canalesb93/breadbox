//go:build integration && !lite

// Integration tests for POST /api/v1/rules/{id}/apply and POST
// /api/v1/rules/apply-all — the previously-uncovered retroactive-apply
// surfaces. Run with:
//
//	DATABASE_URL="postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable" \
//	  go test -tags integration -count=1 -p 1 -v ./internal/api/... -run TestApplyRule_
package api

import (
	"context"
	"net/http"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// applyRulesSeed creates a category, two transactions ("Coffee Shop A" and
// "Gas Station"), and returns their UUIDs and the category slug.
func applyRulesSeed(t *testing.T, env *testEnv) (catSlug string, coffeeID, gasID string) {
	t.Helper()
	cat := testutil.MustCreateCategory(t, env.Queries, "food_and_drink_coffee", "Coffee")
	user := testutil.MustCreateUser(t, env.Queries, "Apply Tester")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "apply_item")
	acct := testutil.MustCreateAccount(t, env.Queries, conn.ID, "apply_acct", "Checking")
	coffee := testutil.MustCreateTransaction(t, env.Queries, acct.ID, "apply_coffee", "Coffee Shop A", 450, "2025-04-10")
	gas := testutil.MustCreateTransaction(t, env.Queries, acct.ID, "apply_gas", "Gas Station", 4500, "2025-04-11")
	return cat.Slug, pgconv.FormatUUID(coffee.ID), pgconv.FormatUUID(gas.ID)
}

// createApplyRule POSTs a rule via the REST handler and returns its ID. Trigger
// defaults to "always" so it counts toward the rule registry; we apply it
// retroactively, which ignores trigger anyway.
func createApplyRule(t *testing.T, env *testEnv, name, categorySlug, fieldValue string) string {
	t.Helper()
	resp := env.doPost(t, "/api/v1/rules", map[string]any{
		"name":          name,
		"category_slug": categorySlug,
		"trigger":       "always",
		"conditions": map[string]any{
			"field": "provider_name",
			"op":    "contains",
			"value": fieldValue,
		},
	})
	if resp.StatusCode != http.StatusCreated {
		body := readBody(t, resp)
		t.Fatalf("createApplyRule(%q): status %d body %s", name, resp.StatusCode, body)
	}
	var out struct {
		ID string `json:"id"`
	}
	parseJSON(t, resp, &out)
	if out.ID == "" {
		t.Fatalf("createApplyRule(%q): no id in response", name)
	}
	return out.ID
}

// TestApplyRule_AppliesToMatching seeds a rule that matches one of two
// transactions, applies it, and asserts the matched txn was recategorized.
func TestApplyRule_AppliesToMatching(t *testing.T) {
	env := setupTestEnv(t)
	catSlug, coffeeID, gasID := applyRulesSeed(t, env)
	ruleID := createApplyRule(t, env, "Coffee → Coffee Cat", catSlug, "Coffee")

	resp := env.doPost(t, "/api/v1/rules/"+ruleID+"/apply", nil)
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		RuleID        string `json:"rule_id"`
		AffectedCount int64  `json:"affected_count"`
	}
	parseJSON(t, resp, &out)
	if out.AffectedCount != 1 {
		t.Fatalf("want affected_count=1 (Coffee Shop A only), got %d", out.AffectedCount)
	}

	// Coffee transaction is now categorized to coffee.
	got, err := env.Service.GetTransaction(context.Background(), coffeeID)
	if err != nil {
		t.Fatalf("get coffee txn: %v", err)
	}
	if got.Category == nil || got.Category.Slug == nil || *got.Category.Slug != catSlug {
		t.Fatalf("want coffee txn categorized to %q, got %+v", catSlug, got.Category)
	}

	// Gas transaction was untouched.
	gasGot, err := env.Service.GetTransaction(context.Background(), gasID)
	if err != nil {
		t.Fatalf("get gas txn: %v", err)
	}
	if gasGot.Category != nil && gasGot.Category.Slug != nil && *gasGot.Category.Slug == catSlug {
		t.Fatalf("gas txn should not have been categorized to coffee, got %+v", gasGot.Category)
	}
}

// TestApplyRule_NoMatches applies a rule whose conditions match no rows.
// Response is 200 with affected_count=0.
func TestApplyRule_NoMatches(t *testing.T) {
	env := setupTestEnv(t)
	catSlug, _, _ := applyRulesSeed(t, env)
	ruleID := createApplyRule(t, env, "NoMatch", catSlug, "ZZZZZZNOSUCH")

	resp := env.doPost(t, "/api/v1/rules/"+ruleID+"/apply", nil)
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		AffectedCount int64 `json:"affected_count"`
	}
	parseJSON(t, resp, &out)
	if out.AffectedCount != 0 {
		t.Fatalf("want affected_count=0, got %d", out.AffectedCount)
	}
}

// TestApplyRule_RespectsCategoryOverride — a transaction with
// category_override=true must NOT be touched by retroactive apply. This is
// the load-bearing "category_override is sacred" assertion.
func TestApplyRule_RespectsCategoryOverride(t *testing.T) {
	env := setupTestEnv(t)
	catSlug, coffeeID, _ := applyRulesSeed(t, env)

	// Pre-set a manual override on the coffee txn to a different category.
	otherCat := testutil.MustCreateCategory(t, env.Queries, "personal_care", "Personal Care")
	if err := env.Service.SetTransactionCategory(context.Background(), coffeeID, otherCat.ShortID, service.SystemActor()); err != nil {
		t.Fatalf("seed override: %v", err)
	}

	// Now create + apply a rule that would otherwise recategorize the coffee txn.
	ruleID := createApplyRule(t, env, "Coffee → Coffee Cat", catSlug, "Coffee")

	resp := env.doPost(t, "/api/v1/rules/"+ruleID+"/apply", nil)
	assertStatus(t, resp, http.StatusOK)

	// The match counter still increments (sync-time parity), but the UPDATE
	// is filtered to category_override=FALSE, so the txn must NOT change.
	got, err := env.Service.GetTransaction(context.Background(), coffeeID)
	if err != nil {
		t.Fatalf("get coffee txn: %v", err)
	}
	if got.Category == nil || got.Category.Slug == nil || *got.Category.Slug != otherCat.Slug {
		t.Fatalf("category_override is sacred: want category %q untouched, got %+v", otherCat.Slug, got.Category)
	}
	if got.CategoryOverride == "none" {
		t.Fatalf("category_override flag must remain true after apply, got false")
	}
}

// TestApplyRule_NotFound — applying a non-existent rule returns 404.
func TestApplyRule_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doPost(t, "/api/v1/rules/00000000-0000-0000-0000-000000000000/apply", nil)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// TestApplyRule_RequiresWriteScope — read-only key is blocked.
func TestApplyRule_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	resp := env.doPost(t, "/api/v1/rules/00000000-0000-0000-0000-000000000000/apply", nil)
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

// TestApplyAllRules_AppliesAllActive seeds two enabled rules with different
// matchers, applies-all, and verifies both transactions are recategorized.
func TestApplyAllRules_AppliesAllActive(t *testing.T) {
	env := setupTestEnv(t)
	coffeeCatSlug, coffeeID, gasID := applyRulesSeed(t, env)
	gasCat := testutil.MustCreateCategory(t, env.Queries, "transportation_gas", "Gas")

	_ = createApplyRule(t, env, "Coffee → coffee", coffeeCatSlug, "Coffee")
	_ = createApplyRule(t, env, "Gas → gas", gasCat.Slug, "Gas")

	resp := env.doPost(t, "/api/v1/rules/apply-all", nil)
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		RulesApplied  map[string]int64 `json:"rules_applied"`
		TotalAffected int64            `json:"total_affected"`
	}
	parseJSON(t, resp, &out)

	if out.TotalAffected < 2 {
		t.Fatalf("want total_affected >= 2 (both rules matched their txns), got %d (rules_applied=%v)", out.TotalAffected, out.RulesApplied)
	}
	if len(out.RulesApplied) < 2 {
		t.Fatalf("want at least 2 rules in rules_applied, got %v", out.RulesApplied)
	}

	// Both txns now have their respective categories.
	gotCoffee, err := env.Service.GetTransaction(context.Background(), coffeeID)
	if err != nil {
		t.Fatalf("get coffee txn: %v", err)
	}
	if gotCoffee.Category == nil || gotCoffee.Category.Slug == nil || *gotCoffee.Category.Slug != coffeeCatSlug {
		t.Fatalf("coffee txn should be %q, got %+v", coffeeCatSlug, gotCoffee.Category)
	}

	gotGas, err := env.Service.GetTransaction(context.Background(), gasID)
	if err != nil {
		t.Fatalf("get gas txn: %v", err)
	}
	if gotGas.Category == nil || gotGas.Category.Slug == nil || *gotGas.Category.Slug != gasCat.Slug {
		t.Fatalf("gas txn should be %q, got %+v", gasCat.Slug, gotGas.Category)
	}
}

// TestApplyAllRules_SkipsDisabled — a disabled rule is not applied even
// when its conditions would match.
func TestApplyAllRules_SkipsDisabled(t *testing.T) {
	env := setupTestEnv(t)
	coffeeCatSlug, coffeeID, _ := applyRulesSeed(t, env)

	ruleID := createApplyRule(t, env, "Coffee → coffee", coffeeCatSlug, "Coffee")

	// Disable the rule via PUT.
	disabled := false
	resp := env.doPut(t, "/api/v1/rules/"+ruleID, map[string]any{"enabled": &disabled})
	assertStatus(t, resp, http.StatusOK)

	resp = env.doPost(t, "/api/v1/rules/apply-all", nil)
	assertStatus(t, resp, http.StatusOK)

	// Coffee txn should NOT be categorized — the rule was skipped.
	gotCoffee, err := env.Service.GetTransaction(context.Background(), coffeeID)
	if err != nil {
		t.Fatalf("get coffee txn: %v", err)
	}
	if gotCoffee.Category != nil && gotCoffee.Category.Slug != nil && *gotCoffee.Category.Slug == coffeeCatSlug {
		t.Fatalf("disabled rule must not apply; coffee txn unexpectedly categorized to %q", coffeeCatSlug)
	}
}

// TestApplyAllRules_RespectsCategoryOverride — txn with category_override=true
// stays untouched even when a matching active rule runs as part of apply-all.
func TestApplyAllRules_RespectsCategoryOverride(t *testing.T) {
	env := setupTestEnv(t)
	coffeeCatSlug, coffeeID, _ := applyRulesSeed(t, env)

	// Pre-set a manual override on the coffee txn.
	otherCat := testutil.MustCreateCategory(t, env.Queries, "personal_care", "Personal Care")
	if err := env.Service.SetTransactionCategory(context.Background(), coffeeID, otherCat.ShortID, service.SystemActor()); err != nil {
		t.Fatalf("seed override: %v", err)
	}

	_ = createApplyRule(t, env, "Coffee → coffee", coffeeCatSlug, "Coffee")

	resp := env.doPost(t, "/api/v1/rules/apply-all", nil)
	assertStatus(t, resp, http.StatusOK)

	gotCoffee, err := env.Service.GetTransaction(context.Background(), coffeeID)
	if err != nil {
		t.Fatalf("get coffee txn: %v", err)
	}
	if gotCoffee.Category == nil || gotCoffee.Category.Slug == nil || *gotCoffee.Category.Slug != otherCat.Slug {
		t.Fatalf("category_override is sacred: want %q, got %+v", otherCat.Slug, gotCoffee.Category)
	}
	if gotCoffee.CategoryOverride == "none" {
		t.Fatalf("category_override flag must remain true after apply-all")
	}
}

// TestApplyAllRules_RequiresWriteScope — read-only key is blocked.
func TestApplyAllRules_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	resp := env.doPost(t, "/api/v1/rules/apply-all", nil)
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}
