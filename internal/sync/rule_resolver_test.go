package sync

import (
	"encoding/json"
	"testing"

	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// Helper to create a pgtype.UUID from a byte value for testing.
func testUUID(b byte) pgtype.UUID {
	var u pgtype.UUID
	u.Bytes[0] = b
	u.Valid = true
	return u
}

// Helper to compile a condition for tests. Panics on error.
func mustCompile(t *testing.T, c *Condition) *compiledCondition {
	t.Helper()
	cc, err := compileCondition(c)
	if err != nil {
		t.Fatalf("compileCondition failed: %v", err)
	}
	return cc
}

func TestEvaluateCondition_SimpleEq(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "name", Op: "eq", Value: "Starbucks"})
	tctx := TransactionContext{Name: "Starbucks"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected eq to match")
	}
}

func TestEvaluateCondition_EqCaseInsensitive(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "name", Op: "eq", Value: "STARBUCKS"})
	tctx := TransactionContext{Name: "starbucks"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected case-insensitive eq to match")
	}
}

func TestEvaluateCondition_Neq(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "name", Op: "neq", Value: "Starbucks"})
	tctx := TransactionContext{Name: "Target"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected neq to match for different values")
	}

	tctx.Name = "Starbucks"
	if evaluateCondition(cc, tctx) {
		t.Error("expected neq to not match for same value")
	}
}

func TestEvaluateCondition_Contains(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "star"})
	tctx := TransactionContext{Name: "Starbucks Coffee"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected contains to match (case-insensitive)")
	}

	tctx.Name = "Target"
	if evaluateCondition(cc, tctx) {
		t.Error("expected contains to not match")
	}
}

func TestEvaluateCondition_NotContains(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "name", Op: "not_contains", Value: "star"})
	tctx := TransactionContext{Name: "Target"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected not_contains to match")
	}

	tctx.Name = "Starbucks"
	if evaluateCondition(cc, tctx) {
		t.Error("expected not_contains to not match")
	}
}

func TestEvaluateCondition_Matches(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "name", Op: "matches", Value: "^Star.*ks$"})
	tctx := TransactionContext{Name: "Starbucks"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected matches to match")
	}

	tctx.Name = "Target"
	if evaluateCondition(cc, tctx) {
		t.Error("expected matches to not match")
	}
}

func TestEvaluateCondition_MatchesInvalidRegex(t *testing.T) {
	_, err := compileCondition(&Condition{Field: "name", Op: "matches", Value: "[invalid"})
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestEvaluateCondition_In(t *testing.T) {
	cc := mustCompile(t, &Condition{
		Field: "name",
		Op:    "in",
		Value: []interface{}{"Starbucks", "Target", "Walmart"},
	})
	tctx := TransactionContext{Name: "target"} // case-insensitive
	if !evaluateCondition(cc, tctx) {
		t.Error("expected in to match (case-insensitive)")
	}

	tctx.Name = "Costco"
	if evaluateCondition(cc, tctx) {
		t.Error("expected in to not match for non-listed value")
	}
}

func TestEvaluateCondition_And(t *testing.T) {
	cc := mustCompile(t, &Condition{
		And: []Condition{
			{Field: "name", Op: "contains", Value: "coffee"},
			{Field: "amount", Op: "gt", Value: float64(5)},
		},
	})

	tctx := TransactionContext{Name: "Coffee Shop", Amount: 10.50}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected AND to match when all conditions pass")
	}

	tctx.Amount = 3.00
	if evaluateCondition(cc, tctx) {
		t.Error("expected AND to fail when one condition fails")
	}
}

func TestEvaluateCondition_Or(t *testing.T) {
	cc := mustCompile(t, &Condition{
		Or: []Condition{
			{Field: "name", Op: "eq", Value: "Starbucks"},
			{Field: "name", Op: "eq", Value: "Dunkin"},
		},
	})

	tctx := TransactionContext{Name: "Dunkin"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected OR to match when one condition passes")
	}

	tctx.Name = "Target"
	if evaluateCondition(cc, tctx) {
		t.Error("expected OR to fail when no conditions pass")
	}
}

func TestEvaluateCondition_Not(t *testing.T) {
	cc := mustCompile(t, &Condition{
		Not: &Condition{Field: "name", Op: "eq", Value: "Starbucks"},
	})

	tctx := TransactionContext{Name: "Target"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected NOT to match when inner condition fails")
	}

	tctx.Name = "Starbucks"
	if evaluateCondition(cc, tctx) {
		t.Error("expected NOT to fail when inner condition passes")
	}
}

func TestEvaluateCondition_NestedAndInsideOr(t *testing.T) {
	// OR(AND(name contains "coffee", amount > 5), name eq "Starbucks")
	cc := mustCompile(t, &Condition{
		Or: []Condition{
			{
				And: []Condition{
					{Field: "name", Op: "contains", Value: "coffee"},
					{Field: "amount", Op: "gt", Value: float64(5)},
				},
			},
			{Field: "name", Op: "eq", Value: "Starbucks"},
		},
	})

	// Matches second OR branch
	tctx := TransactionContext{Name: "Starbucks", Amount: 2.00}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected nested condition to match via second OR branch")
	}

	// Matches first OR branch (AND)
	tctx = TransactionContext{Name: "Coffee Shop", Amount: 10.00}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected nested condition to match via first OR branch (AND)")
	}

	// No match
	tctx = TransactionContext{Name: "Coffee Shop", Amount: 3.00}
	if evaluateCondition(cc, tctx) {
		t.Error("expected nested condition to fail when no branch matches")
	}
}

func TestEvaluateCondition_NumericGte(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "amount", Op: "gte", Value: float64(100)})

	tctx := TransactionContext{Amount: 100}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected gte to match for equal value")
	}

	tctx.Amount = 150
	if !evaluateCondition(cc, tctx) {
		t.Error("expected gte to match for greater value")
	}

	tctx.Amount = 99.99
	if evaluateCondition(cc, tctx) {
		t.Error("expected gte to fail for lesser value")
	}
}

func TestEvaluateCondition_NumericLte(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "amount", Op: "lte", Value: float64(50)})

	tctx := TransactionContext{Amount: 50}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected lte to match for equal value")
	}

	tctx.Amount = 25
	if !evaluateCondition(cc, tctx) {
		t.Error("expected lte to match for lesser value")
	}

	tctx.Amount = 50.01
	if evaluateCondition(cc, tctx) {
		t.Error("expected lte to fail for greater value")
	}
}

func TestEvaluateCondition_NumericLt(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "amount", Op: "lt", Value: float64(10)})

	tctx := TransactionContext{Amount: 9.99}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected lt to match")
	}

	tctx.Amount = 10
	if evaluateCondition(cc, tctx) {
		t.Error("expected lt to fail for equal value")
	}
}

func TestEvaluateCondition_NumericGt(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "amount", Op: "gt", Value: float64(10)})

	tctx := TransactionContext{Amount: 10.01}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected gt to match")
	}

	tctx.Amount = 10
	if evaluateCondition(cc, tctx) {
		t.Error("expected gt to fail for equal value")
	}
}

func TestEvaluateCondition_NumericEq(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "amount", Op: "eq", Value: float64(42.50)})

	tctx := TransactionContext{Amount: 42.50}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected numeric eq to match")
	}

	tctx.Amount = 42.51
	if evaluateCondition(cc, tctx) {
		t.Error("expected numeric eq to fail for different value")
	}
}

func TestEvaluateCondition_BoolEq(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "pending", Op: "eq", Value: true})

	tctx := TransactionContext{Pending: true}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected bool eq to match")
	}

	tctx.Pending = false
	if evaluateCondition(cc, tctx) {
		t.Error("expected bool eq to fail")
	}
}

func TestEvaluateCondition_BoolNeq(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "pending", Op: "neq", Value: true})

	tctx := TransactionContext{Pending: false}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected bool neq to match when values differ")
	}
}

func TestEvaluateCondition_MerchantName(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "merchant_name", Op: "contains", Value: "amazon"})

	tctx := TransactionContext{MerchantName: "Amazon.com"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected merchant_name contains to match")
	}

	tctx.MerchantName = ""
	if evaluateCondition(cc, tctx) {
		t.Error("expected merchant_name contains to fail for empty string")
	}
}

func TestEvaluateCondition_ProviderField(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "provider", Op: "eq", Value: "plaid"})

	tctx := TransactionContext{Provider: "plaid"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected provider eq to match")
	}

	tctx.Provider = "teller"
	if evaluateCondition(cc, tctx) {
		t.Error("expected provider eq to fail for different provider")
	}
}

func TestEvaluateCondition_AccountIDField(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "account_id", Op: "eq", Value: "abc-123"})

	tctx := TransactionContext{AccountID: "abc-123"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected account_id eq to match")
	}
}

func TestEvaluateCondition_UserIDField(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "user_id", Op: "eq", Value: "user-456"})

	tctx := TransactionContext{UserID: "user-456"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected user_id eq to match")
	}
}

func TestEvaluateCondition_UnknownField(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "unknown", Op: "eq", Value: "test"})

	tctx := TransactionContext{Name: "test"}
	if evaluateCondition(cc, tctx) {
		t.Error("expected unknown field to return false")
	}
}

func TestEvaluateCondition_UnknownOp(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "name", Op: "regex_match", Value: "test"})

	tctx := TransactionContext{Name: "test"}
	if evaluateCondition(cc, tctx) {
		t.Error("expected unknown op to return false")
	}
}

func TestEvaluateCondition_EmptyFieldValue(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "merchant_name", Op: "contains", Value: "coffee"})

	// Empty merchant_name should not match "contains coffee"
	tctx := TransactionContext{MerchantName: ""}
	if evaluateCondition(cc, tctx) {
		t.Error("expected empty field to not match contains")
	}
}

func TestEvaluateCondition_EmptyCondition(t *testing.T) {
	// An empty Condition{} compiles to nil and evaluates to true (match-all),
	// matching the "NULL conditions == match every transaction" DB semantic.
	cc := mustCompile(t, &Condition{})
	if cc != nil {
		t.Fatalf("expected empty condition to compile to nil, got %+v", cc)
	}
	tctx := TransactionContext{Name: "anything"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected nil compiled condition to evaluate as match-all (true)")
	}
}

func TestEvaluateCondition_CategoryPrimary(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "category_primary", Op: "eq", Value: "FOOD_AND_DRINK"})

	tctx := TransactionContext{CategoryPrimary: "food_and_drink"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected category_primary eq to match (case-insensitive)")
	}
}

func TestEvaluateCondition_CategoryDetailed(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "category_detailed", Op: "contains", Value: "groceries"})

	tctx := TransactionContext{CategoryDetailed: "FOOD_AND_DRINK_GROCERIES"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected category_detailed contains to match")
	}
}

// --- Baseline regression tests (audit §D) ---
//
// These lock down current semantics so upcoming resolver-chaining / priority-
// inversion work can't silently regress them.

func TestEvaluateCondition_Tags_Contains(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "tags", Op: "contains", Value: "needs-review"})

	tctx := TransactionContext{Tags: []string{"coffee", "needs-review"}}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected tags contains to match when slug present")
	}

	tctx.Tags = []string{"coffee"}
	if evaluateCondition(cc, tctx) {
		t.Error("expected tags contains to not match when slug absent")
	}

	// Case-insensitive.
	tctx.Tags = []string{"NEEDS-REVIEW"}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected tags contains to be case-insensitive")
	}
}

func TestEvaluateCondition_Tags_NotContains(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "tags", Op: "not_contains", Value: "needs-review"})

	tctx := TransactionContext{Tags: []string{"coffee"}}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected tags not_contains to match when slug absent")
	}

	tctx.Tags = []string{"coffee", "needs-review"}
	if evaluateCondition(cc, tctx) {
		t.Error("expected tags not_contains to not match when slug present")
	}

	// Nil tags slice treated as no tags.
	tctx.Tags = nil
	if !evaluateCondition(cc, tctx) {
		t.Error("expected tags not_contains to match on nil tag slice")
	}
}

func TestEvaluateCondition_Tags_In(t *testing.T) {
	cc := mustCompile(t, &Condition{
		Field: "tags",
		Op:    "in",
		Value: []interface{}{"needs-review", "flagged"},
	})

	tctx := TransactionContext{Tags: []string{"flagged"}}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected tags in to match when any slug is present")
	}

	tctx.Tags = []string{"coffee"}
	if evaluateCondition(cc, tctx) {
		t.Error("expected tags in to not match when no slug is present")
	}
}

func TestEvaluateCondition_SingleConditionAnd(t *testing.T) {
	// {and: [one]} should behave like one.
	cc := mustCompile(t, &Condition{
		And: []Condition{
			{Field: "name", Op: "eq", Value: "Starbucks"},
		},
	})

	if !evaluateCondition(cc, TransactionContext{Name: "Starbucks"}) {
		t.Error("expected single-child AND to match")
	}
	if evaluateCondition(cc, TransactionContext{Name: "Target"}) {
		t.Error("expected single-child AND to not match")
	}
}

func TestEvaluateCondition_DeeplyNestedAndOfOrOfNot(t *testing.T) {
	// AND( OR( NOT(pending eq true), name eq "X" ), amount gt 0 )
	cc := mustCompile(t, &Condition{
		And: []Condition{
			{
				Or: []Condition{
					{Not: &Condition{Field: "pending", Op: "eq", Value: true}},
					{Field: "name", Op: "eq", Value: "X"},
				},
			},
			{Field: "amount", Op: "gt", Value: float64(0)},
		},
	})

	// Matches: pending=false (NOT true) + amount > 0
	if !evaluateCondition(cc, TransactionContext{Pending: false, Amount: 10}) {
		t.Error("expected deeply nested condition to match")
	}
	// Fails: pending=true + name≠X + amount > 0 → OR branch false
	if evaluateCondition(cc, TransactionContext{Pending: true, Name: "Y", Amount: 10}) {
		t.Error("expected deeply nested condition to fail when OR has no true branch")
	}
	// Fails: pending=false + amount <= 0 → AND fails on second clause
	if evaluateCondition(cc, TransactionContext{Pending: false, Amount: 0}) {
		t.Error("expected deeply nested condition to fail when outer AND fails")
	}
}

func TestResolveWithContext_MultipleActionsOneRule(t *testing.T) {
	ruleID := testUUID(10)
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:      ruleID,
				shortID: "rule001x",
				name:    "combo",
				actions: []typedAction{
					{Type: "set_category", CategorySlug: "catA"},
					{Type: "add_tag", TagSlug: "tagA"},
					{Type: "add_comment", Content: "hello"},
				},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "coffee"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	result := r.ResolveWithContext("plaid", TransactionContext{Name: "Coffee Shop"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.CategorySlug != "catA" {
		t.Errorf("expected category catA, got %q", result.CategorySlug)
	}
	if len(result.TagsToAdd) != 1 || result.TagsToAdd[0] != "tagA" {
		t.Errorf("expected tags [tagA], got %v", result.TagsToAdd)
	}
	if len(result.Comments) != 1 || result.Comments[0] != "hello" {
		t.Errorf("expected comments [hello], got %v", result.Comments)
	}
	// Single rule produces three sources (one per action).
	if len(result.Sources) != 3 {
		t.Errorf("expected 3 sources, got %d (%+v)", len(result.Sources), result.Sources)
	}
	if r.hitCounts[ruleID.Bytes] != 1 {
		t.Errorf("expected hit count 1, got %d", r.hitCounts[ruleID.Bytes])
	}
}

func TestResolveWithContext_AddTagDedupAcrossRules(t *testing.T) {
	// Two rules both add the same tag slug — TagsToAdd should dedup.
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(10),
				actions:   []typedAction{{Type: "add_tag", TagSlug: "shared"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "x"}),
			},
			{
				id:        testUUID(11),
				actions:   []typedAction{{Type: "add_tag", TagSlug: "shared"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider", Op: "eq", Value: "plaid"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	result := r.ResolveWithContext("plaid", TransactionContext{Name: "x thing", Provider: "plaid"}, true)
	if result == nil {
		t.Fatal("expected result")
	}
	if len(result.TagsToAdd) != 1 {
		t.Errorf("expected 1 tag (deduped), got %v", result.TagsToAdd)
	}
}

func TestResolveWithContext_AddTagAccumulationAcrossRules(t *testing.T) {
	// Two rules adding different tags — both should land.
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(10),
				actions:   []typedAction{{Type: "add_tag", TagSlug: "one"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "x"}),
			},
			{
				id:        testUUID(11),
				actions:   []typedAction{{Type: "add_tag", TagSlug: "two"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider", Op: "eq", Value: "plaid"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	result := r.ResolveWithContext("plaid", TransactionContext{Name: "x thing", Provider: "plaid"}, true)
	if result == nil {
		t.Fatal("expected result")
	}
	if len(result.TagsToAdd) != 2 {
		t.Errorf("expected 2 tags accumulated, got %v", result.TagsToAdd)
	}
}

func TestResolveWithContext_AddCommentAccumulation(t *testing.T) {
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(10),
				actions:   []typedAction{{Type: "add_comment", Content: "first"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "x"}),
			},
			{
				id:        testUUID(11),
				actions:   []typedAction{{Type: "add_comment", Content: "second"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider", Op: "eq", Value: "plaid"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	result := r.ResolveWithContext("plaid", TransactionContext{Name: "x thing", Provider: "plaid"}, true)
	if result == nil {
		t.Fatal("expected result")
	}
	if len(result.Comments) != 2 || result.Comments[0] != "first" || result.Comments[1] != "second" {
		t.Errorf("expected both comments in order, got %v", result.Comments)
	}
}

func TestEvaluateCondition_CategoryAndAccountName(t *testing.T) {
	// category (assigned) and account_name are Phase 1c additions. Both are
	// plain string fields using the standard string operators.
	tctx := TransactionContext{
		Category:    "food_and_drink_coffee",
		AccountName: "Chase Freedom Checking",
	}

	cc := mustCompile(t, &Condition{Field: "category", Op: "eq", Value: "food_and_drink_coffee"})
	if !evaluateCondition(cc, tctx) {
		t.Error("expected category eq to match")
	}

	cc = mustCompile(t, &Condition{Field: "account_name", Op: "contains", Value: "checking"})
	if !evaluateCondition(cc, tctx) {
		t.Error("expected account_name contains to match (case-insensitive)")
	}

	cc = mustCompile(t, &Condition{Field: "account_name", Op: "neq", Value: "Savings"})
	if !evaluateCondition(cc, tctx) {
		t.Error("expected account_name neq to match when different")
	}
}

func TestResolveWithContext_ChainingCategoryVisibleToLaterRule(t *testing.T) {
	// Rule A (earlier stage) sets category to "coffee". Rule B (later stage)
	// conditions on `category eq "coffee"` — it should observe the mutation.
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(10),
				actions:   []typedAction{{Type: "set_category", CategorySlug: "food_and_drink_coffee"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "starbucks"}),
			},
			{
				id:        testUUID(11),
				actions:   []typedAction{{Type: "add_tag", TagSlug: "dining"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "category", Op: "eq", Value: "food_and_drink_coffee"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	result := r.ResolveWithContext("plaid", TransactionContext{Name: "Starbucks 123"}, true)
	if result == nil {
		t.Fatal("expected result")
	}
	if result.CategorySlug != "food_and_drink_coffee" {
		t.Errorf("expected category set by rule A, got %q", result.CategorySlug)
	}
	if len(result.TagsToAdd) != 1 || result.TagsToAdd[0] != "dining" {
		t.Errorf("expected later rule to observe rule A's category and add 'dining' tag, got %v", result.TagsToAdd)
	}
}

func TestResolveWithContext_ChainingTagsVisibleToLaterRule(t *testing.T) {
	// Rule A (earlier stage) adds tag "coffee".
	// Rule B (later stage) has condition `tags contains "coffee"` — it
	// should observe the tag that rule A just added, even though the
	// incoming transaction carried no tags.
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(10), // earlier
				actions:   []typedAction{{Type: "add_tag", TagSlug: "coffee"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "starbucks"}),
			},
			{
				id:      testUUID(11), // later
				actions: []typedAction{{Type: "set_category", CategorySlug: "food_and_drink_coffee"}},
				trigger: "always",
				condition: mustCompile(t, &Condition{
					Field: "tags", Op: "contains", Value: "coffee",
				}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	tctx := TransactionContext{Name: "STARBUCKS #1234", Provider: "plaid"} // Tags nil
	result := r.ResolveWithContext("plaid", tctx, true)

	if result == nil {
		t.Fatal("expected chained result")
	}
	if len(result.TagsToAdd) != 1 || result.TagsToAdd[0] != "coffee" {
		t.Errorf("expected tags [coffee], got %v", result.TagsToAdd)
	}
	if result.CategorySlug != "food_and_drink_coffee" {
		t.Errorf("expected later rule to observe earlier rule's tag, got category %q", result.CategorySlug)
	}
}

func TestResolveWithContext_ChainingDoesNotLeakIntoCallerContext(t *testing.T) {
	// The caller's TransactionContext (passed by value) should not reflect
	// resolver-internal mutations. In particular, Tags mid-resolve append
	// should not leak back if the caller kept a reference to the slice.
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(10),
				actions:   []typedAction{{Type: "add_tag", TagSlug: "new-tag"}},
				trigger:   "always",
				condition: nil, // match-all
			},
		},
		uncategorizedID: testUUID(99),
	}

	originalTags := []string{"pre-existing"}
	tctx := TransactionContext{Tags: originalTags}
	_ = r.ResolveWithContext("plaid", tctx, true)

	// originalTags slice must not have been appended to.
	if len(originalTags) != 1 || originalTags[0] != "pre-existing" {
		t.Errorf("expected caller's Tags slice to be untouched, got %v", originalTags)
	}
	// Caller's tctx.Tags header also unchanged (pointer identity preserved).
	if len(tctx.Tags) != 1 {
		t.Errorf("expected caller's tctx.Tags len=1, got %d", len(tctx.Tags))
	}
}

func TestResolveWithContext_RemoveTag_PresentOnTransaction(t *testing.T) {
	// Transaction carries `needs-review`. Rule's remove_tag deletes it.
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(10),
				actions:   []typedAction{{Type: "remove_tag", TagSlug: "needs-review"}},
				trigger:   "always",
				condition: nil, // match-all
			},
		},
		uncategorizedID: testUUID(99),
	}

	tctx := TransactionContext{Tags: []string{"needs-review", "other"}}
	result := r.ResolveWithContext("plaid", tctx, false)
	if result == nil {
		t.Fatal("expected result")
	}
	if len(result.TagsToRemove) != 1 || result.TagsToRemove[0] != "needs-review" {
		t.Errorf("expected TagsToRemove=[needs-review], got %v", result.TagsToRemove)
	}
	if len(result.TagsToAdd) != 0 {
		t.Errorf("expected no adds, got %v", result.TagsToAdd)
	}
}

func TestResolveWithContext_RemoveTag_NotPresentIsNoOp(t *testing.T) {
	// Transaction has no tags. Rule's remove_tag is a no-op.
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(10),
				actions:   []typedAction{{Type: "remove_tag", TagSlug: "needs-review"}},
				trigger:   "always",
				condition: nil,
			},
		},
		uncategorizedID: testUUID(99),
	}

	result := r.ResolveWithContext("plaid", TransactionContext{}, true)
	// Rule matched (hit_count bumped), but produced no side-effect.
	if result == nil {
		t.Fatal("expected result with hit_count bump")
	}
	if len(result.TagsToRemove) != 0 {
		t.Errorf("expected no removes for absent slug, got %v", result.TagsToRemove)
	}
}

func TestResolveWithContext_AddThenRemoveCancelsOut(t *testing.T) {
	// Earlier rule adds "coffee"; later rule removes it. Net: no DB write.
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(10),
				actions:   []typedAction{{Type: "add_tag", TagSlug: "coffee"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "starbucks"}),
			},
			{
				id:        testUUID(11),
				actions:   []typedAction{{Type: "remove_tag", TagSlug: "coffee"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "amount", Op: "lt", Value: float64(1)}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	// Both rules match; they should cancel.
	tctx := TransactionContext{Name: "Starbucks", Amount: 0.50}
	result := r.ResolveWithContext("plaid", tctx, true)
	if result == nil {
		t.Fatal("expected result with hits")
	}
	if len(result.TagsToAdd) != 0 {
		t.Errorf("expected TagsToAdd cancelled, got %v", result.TagsToAdd)
	}
	if len(result.TagsToRemove) != 0 {
		t.Errorf("expected TagsToRemove cancelled, got %v", result.TagsToRemove)
	}
	// No tag-related sources should remain.
	for _, s := range result.Sources {
		if s.ActionField == "tag" || s.ActionField == "tag_remove" {
			t.Errorf("unexpected cancelled-action source: %+v", s)
		}
	}
}

func TestResolveWithContext_RemoveThenAddReAdds(t *testing.T) {
	// Earlier rule removes tag; later rule re-adds same tag. Net: tag present.
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(10),
				actions:   []typedAction{{Type: "remove_tag", TagSlug: "needs-review"}},
				trigger:   "always",
				condition: nil,
			},
			{
				id:        testUUID(11),
				actions:   []typedAction{{Type: "add_tag", TagSlug: "needs-review"}},
				trigger:   "always",
				condition: nil,
			},
		},
		uncategorizedID: testUUID(99),
	}

	tctx := TransactionContext{Tags: []string{"needs-review"}}
	result := r.ResolveWithContext("plaid", tctx, false)
	if result == nil {
		t.Fatal("expected result")
	}
	// Remove cancelled by re-add; the final state should preserve the tag
	// without any DB writes (nothing added, nothing removed — the prior
	// remove's queued delete is dropped when the later add fires).
	if len(result.TagsToRemove) != 0 {
		t.Errorf("expected TagsToRemove cancelled, got %v", result.TagsToRemove)
	}
	// The add might still be queued if it's genuinely a "new" add. Under
	// our semantics, add checks for presence in tctx.Tags first — the
	// earlier remove stripped it from tctx.Tags, so the add sees it as
	// absent and queues a new insert. That's reasonable: we re-add.
	if len(result.TagsToAdd) != 1 || result.TagsToAdd[0] != "needs-review" {
		t.Errorf("expected TagsToAdd=[needs-review] as re-add, got %v", result.TagsToAdd)
	}
}

func TestResolveWithContext_RemoveTagVisibleToLaterRule(t *testing.T) {
	// Rule A removes "needs-review". Rule B conditions on `tags not_contains "needs-review"`
	// — it should observe the removal and fire.
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(10),
				actions:   []typedAction{{Type: "remove_tag", TagSlug: "needs-review"}},
				trigger:   "always",
				condition: nil,
			},
			{
				id:        testUUID(11),
				actions:   []typedAction{{Type: "set_category", CategorySlug: "reviewed"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{
					Field: "tags", Op: "not_contains", Value: "needs-review",
				}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	tctx := TransactionContext{Tags: []string{"needs-review"}}
	result := r.ResolveWithContext("plaid", tctx, false)
	if result == nil {
		t.Fatal("expected result")
	}
	if result.CategorySlug != "reviewed" {
		t.Errorf("expected later rule to observe removal and set category, got %q", result.CategorySlug)
	}
}

func TestResolveWithContext_ChainingExistingTagsVisible(t *testing.T) {
	// A re-synced transaction already carries `needs-review`. A rule whose
	// condition asks `tags contains "needs-review"` should match it.
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:      testUUID(10),
				actions: []typedAction{{Type: "set_category", CategorySlug: "flagged"}},
				trigger: "always",
				condition: mustCompile(t, &Condition{
					Field: "tags", Op: "contains", Value: "needs-review",
				}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	tctx := TransactionContext{Name: "X", Tags: []string{"needs-review"}}
	result := r.ResolveWithContext("plaid", tctx, true)
	if result == nil || result.CategorySlug != "flagged" {
		t.Errorf("expected rule to fire on existing tag, got %+v", result)
	}
}

func TestResolveWithContext_TriggerMatrix(t *testing.T) {
	// Matrix: trigger × isNew. Each cell checks whether the rule fires.
	cases := []struct {
		trigger     string
		isNew       bool
		shouldMatch bool
	}{
		{"on_create", true, true},
		{"on_create", false, false},
		{"on_change", true, false},
		{"on_change", false, true},
		{"on_update", true, false}, // legacy alias — same behavior as on_change
		{"on_update", false, true},
		{"always", true, true},
		{"always", false, true},
		{"", true, true},   // default → on_create
		{"", false, false}, // default → on_create
	}

	for _, tc := range cases {
		trigger := tc.trigger
		if trigger == "" {
			trigger = "on_create" // loadRules normalizes this; mirror it here
		}
		r := &RuleResolver{
			hitCounts: make(map[[16]byte]int),
			rules: []compiledRule{
				{
					id:        testUUID(10),
					actions:   []typedAction{{Type: "set_category", CategorySlug: "c"}},
					trigger:   trigger,
					condition: nil, // match-all
				},
			},
			uncategorizedID: testUUID(99),
		}

		result := r.ResolveWithContext("plaid", TransactionContext{}, tc.isNew)
		matched := result != nil
		if matched != tc.shouldMatch {
			t.Errorf("trigger=%q isNew=%v: matched=%v want=%v", tc.trigger, tc.isNew, matched, tc.shouldMatch)
		}
	}
}

func TestResolveWithContext_PriorityOrdering_LastWins(t *testing.T) {
	// Rules are ordered in the slice as loadRules would return them after
	// ORDER BY priority ASC, created_at ASC — so the LATER entry is the
	// higher-priority rule. Under last-writer-wins (Phase 1a chaining),
	// the later rule's set_category wins.
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(10), // lower priority — runs first
				actions:   []typedAction{{Type: "set_category", CategorySlug: "catA"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "coffee"}),
			},
			{
				id:        testUUID(11), // higher priority — runs last, wins
				actions:   []typedAction{{Type: "set_category", CategorySlug: "catB"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "coffee"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	tctx := TransactionContext{Name: "Coffee Shop", Provider: "plaid"}
	result := r.ResolveWithContext("plaid", tctx, true)
	if result == nil || result.CategorySlug != "catB" {
		t.Errorf("expected higher-priority (later) rule's category to win, got %+v", result)
	}
	// Only the winning rule's source should remain for the category field.
	var catSources int
	for _, s := range result.Sources {
		if s.ActionField == "category" {
			catSources++
			if s.ActionValue != "catB" {
				t.Errorf("expected remaining category source to be catB, got %q", s.ActionValue)
			}
		}
	}
	if catSources != 1 {
		t.Errorf("expected exactly 1 category source after last-wins merge, got %d", catSources)
	}
}

func TestResolveWithContext_NoRuleMatchFallsBackToUncategorized(t *testing.T) {
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        testUUID(10),
				actions:   []typedAction{{Type: "set_category", CategorySlug: "catX"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "name", Op: "eq", Value: "Starbucks"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	// Rule doesn't match, should return nil.
	tctx := TransactionContext{
		Name:            "Grocery Store",
		CategoryPrimary: "FOOD_AND_DRINK",
		Provider:        "plaid",
	}
	result := r.ResolveWithContext("plaid", tctx, true)
	if result != nil {
		t.Errorf("expected nil for no match, got %v", result)
	}
}

func TestResolveWithContext_FallbackToUncategorized(t *testing.T) {
	uncatID := testUUID(99)

	r := &RuleResolver{
		hitCounts:       make(map[[16]byte]int),
		rules:           nil, // no rules
		uncategorizedID: uncatID,
	}

	tctx := TransactionContext{
		Name:     "Random Store",
		Provider: "plaid",
	}
	result := r.ResolveWithContext("plaid", tctx, true)
	if result != nil {
		t.Errorf("expected nil for no match, got %v", result)
	}
}

func TestResolveWithContext_HitCountTracking(t *testing.T) {
	ruleID := testUUID(10)

	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        ruleID,
				actions:   []typedAction{{Type: "set_category", CategorySlug: "catA"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "coffee"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	// Two matching transactions.
	tctx := TransactionContext{Name: "Coffee Shop", Provider: "plaid"}
	r.ResolveWithContext("plaid", tctx, true)
	r.ResolveWithContext("plaid", tctx, true)

	if r.hitCounts[ruleID.Bytes] != 2 {
		t.Errorf("expected 2 hits, got %d", r.hitCounts[ruleID.Bytes])
	}
}

func TestResolveWithContext_MergeNonConflictingActions(t *testing.T) {
	ruleAID := testUUID(10)
	ruleBID := testUUID(11)

	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				// Earlier-stage (lower priority) rule — runs first.
				id:        ruleAID,
				actions:   []typedAction{{Type: "set_category", CategorySlug: "catA"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "coffee"}),
			},
			{
				// Later-stage (higher priority) rule — runs last and wins
				// set_category under pipeline-stage semantics.
				id:        ruleBID,
				actions:   []typedAction{{Type: "set_category", CategorySlug: "catB"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider", Op: "eq", Value: "plaid"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	tctx := TransactionContext{Name: "Coffee Shop", Provider: "plaid"}
	result := r.ResolveWithContext("plaid", tctx, true)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.CategorySlug != "catB" {
		t.Errorf("expected last-wins category, got %v", result.CategorySlug)
	}
	// Both rules match, so both bump hit_count even though rule A's
	// set_category was superseded.
	if r.hitCounts[ruleAID.Bytes] != 1 {
		t.Errorf("expected 1 hit for rule A, got %d", r.hitCounts[ruleAID.Bytes])
	}
	if r.hitCounts[ruleBID.Bytes] != 1 {
		t.Errorf("expected 1 hit for rule B, got %d", r.hitCounts[ruleBID.Bytes])
	}
}

func TestResolveWithContext_NoShortCircuit(t *testing.T) {
	ruleAID := testUUID(10)
	ruleBID := testUUID(11)

	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        ruleAID,
				actions:   []typedAction{{Type: "set_category", CategorySlug: "catA"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "coffee"}),
			},
			{
				// Rule B matches but has no actions (future: could set another field)
				id:        ruleBID,
				actions:   nil,
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider", Op: "eq", Value: "plaid"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	tctx := TransactionContext{Name: "Coffee Shop", Provider: "plaid"}
	result := r.ResolveWithContext("plaid", tctx, true)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.CategorySlug != "catA" {
		t.Errorf("expected category from rule A, got %v", result.CategorySlug)
	}
	// Both rules matched and got hits (no short-circuit)
	if r.hitCounts[ruleAID.Bytes] != 1 {
		t.Errorf("expected 1 hit for rule A, got %d", r.hitCounts[ruleAID.Bytes])
	}
	if r.hitCounts[ruleBID.Bytes] != 1 {
		t.Errorf("expected 1 hit for rule B, got %d", r.hitCounts[ruleBID.Bytes])
	}
}

func TestCompileCondition_InvalidRegex(t *testing.T) {
	_, err := compileCondition(&Condition{
		Field: "name",
		Op:    "matches",
		Value: "[unclosed",
	})
	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}

func TestCompileCondition_MatchesNonString(t *testing.T) {
	_, err := compileCondition(&Condition{
		Field: "name",
		Op:    "matches",
		Value: 123,
	})
	if err == nil {
		t.Error("expected error for non-string regex value")
	}
}

func TestEvaluateCondition_AndShortCircuit(t *testing.T) {
	// First condition fails, second should not be reached.
	cc := mustCompile(t, &Condition{
		And: []Condition{
			{Field: "name", Op: "eq", Value: "nope"},
			{Field: "amount", Op: "gt", Value: float64(0)},
		},
	})

	tctx := TransactionContext{Name: "something", Amount: 100}
	if evaluateCondition(cc, tctx) {
		t.Error("expected AND to short-circuit on first false")
	}
}

func TestEvaluateCondition_OrShortCircuit(t *testing.T) {
	// First condition passes, second should not need evaluation.
	cc := mustCompile(t, &Condition{
		Or: []Condition{
			{Field: "name", Op: "eq", Value: "match"},
			{Field: "amount", Op: "gt", Value: float64(1000)},
		},
	})

	tctx := TransactionContext{Name: "match", Amount: 5}
	if !evaluateCondition(cc, tctx) {
		t.Error("expected OR to short-circuit on first true")
	}
}


func TestToString(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected string
	}{
		{nil, ""},
		{"hello", "hello"},
		{float64(42), "42"},
		{float64(3.14), "3.14"},
		{true, "true"},
		{false, "false"},
	}

	for _, tt := range tests {
		result := toString(tt.input)
		if result != tt.expected {
			t.Errorf("toString(%v) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected float64
	}{
		{nil, 0},
		{float64(42.5), 42.5},
		{int(10), 10},
		{int64(20), 20},
		{"3.14", 3.14},
		{"invalid", 0},
	}

	for _, tt := range tests {
		result := toFloat64(tt.input)
		if result != tt.expected {
			t.Errorf("toFloat64(%v) = %f, want %f", tt.input, result, tt.expected)
		}
	}
}

func TestToBool(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected bool
	}{
		{nil, false},
		{true, true},
		{false, false},
		{"true", true},
		{"TRUE", true},
		{"false", false},
		{float64(1), true},
		{float64(0), false},
	}

	for _, tt := range tests {
		result := toBool(tt.input)
		if result != tt.expected {
			t.Errorf("toBool(%v) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestStringInList(t *testing.T) {
	// List of interface{}
	list := []interface{}{"Apple", "Banana", "Cherry"}
	if !stringInList("banana", list) {
		t.Error("expected case-insensitive match in list")
	}
	if stringInList("grape", list) {
		t.Error("expected no match for unlisted value")
	}

	// Nil
	if stringInList("test", nil) {
		t.Error("expected false for nil value")
	}

	// Single value (not a list)
	if !stringInList("hello", "Hello") {
		t.Error("expected single value match")
	}
}

func TestHitCountsJSON_Empty(t *testing.T) {
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
	}

	result := r.HitCountsJSON()
	if result != nil {
		t.Errorf("expected nil for empty hit counts, got %s", string(result))
	}
}

func TestHitCountsJSON_WithHits(t *testing.T) {
	ruleA := testUUID(10)
	ruleB := testUUID(20)

	r := &RuleResolver{
		hitCounts: map[[16]byte]int{
			ruleA.Bytes: 5,
			ruleB.Bytes: 3,
		},
	}

	result := r.HitCountsJSON()
	if result == nil {
		t.Fatal("expected non-nil JSON, got nil")
	}

	var parsed map[string]int
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to unmarshal hit counts JSON: %v", err)
	}

	if len(parsed) != 2 {
		t.Errorf("expected 2 entries, got %d", len(parsed))
	}

	ruleAID := pgconv.FormatUUID(ruleA)
	ruleBID := pgconv.FormatUUID(ruleB)

	if parsed[ruleAID] != 5 {
		t.Errorf("expected 5 hits for rule A, got %d", parsed[ruleAID])
	}
	if parsed[ruleBID] != 3 {
		t.Errorf("expected 3 hits for rule B, got %d", parsed[ruleBID])
	}
}

func TestHitCountsJSON_IntegrationWithResolveWithContext(t *testing.T) {
	ruleID := testUUID(10)

	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:        ruleID,
				actions:   []typedAction{{Type: "set_category", CategorySlug: "catA"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "coffee"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	// Trigger hits.
	tctx := TransactionContext{Name: "Coffee Shop", Provider: "plaid"}
	r.ResolveWithContext("plaid", tctx, true)
	r.ResolveWithContext("plaid", tctx, true)
	r.ResolveWithContext("plaid", tctx, true)

	result := r.HitCountsJSON()
	if result == nil {
		t.Fatal("expected non-nil JSON after resolving")
	}

	var parsed map[string]int
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	ruleIDStr := pgconv.FormatUUID(ruleID)
	if parsed[ruleIDStr] != 3 {
		t.Errorf("expected 3 hits, got %d", parsed[ruleIDStr])
	}
}
