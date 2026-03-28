package sync

import (
	"encoding/json"
	"testing"

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
	// A condition with no field, no AND, no OR, no NOT is an empty leaf.
	// An empty leaf with no op should return false (unknown op).
	cc := mustCompile(t, &Condition{})
	tctx := TransactionContext{Name: "anything"}
	if evaluateCondition(cc, tctx) {
		t.Error("expected empty condition leaf to return false")
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

func TestResolveWithContext_PriorityOrdering(t *testing.T) {
	catA := testUUID(1)
	catB := testUUID(2)

	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:         testUUID(10),
				categoryID: catA,
				condition:  mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "coffee"}),
			},
			{
				id:         testUUID(11),
				categoryID: catB,
				condition:  mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "coffee"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	// Both rules match, but the first (higher priority) wins.
	tctx := TransactionContext{Name: "Coffee Shop", Provider: "plaid"}
	result := r.ResolveWithContext("plaid", tctx)
	if result != catA {
		t.Errorf("expected higher priority rule to win, got %v", result)
	}
}

func TestResolveWithContext_NoRuleMatchFallsBackToUncategorized(t *testing.T) {
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:         testUUID(10),
				categoryID: testUUID(1),
				condition:  mustCompile(t, &Condition{Field: "name", Op: "eq", Value: "Starbucks"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	// Rule doesn't match, should fall back to uncategorized.
	tctx := TransactionContext{
		Name:            "Grocery Store",
		CategoryPrimary: "FOOD_AND_DRINK",
		Provider:        "plaid",
	}
	result := r.ResolveWithContext("plaid", tctx)
	if result != testUUID(99) {
		t.Errorf("expected uncategorized fallback, got %v", result)
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
	result := r.ResolveWithContext("plaid", tctx)
	if result != uncatID {
		t.Errorf("expected uncategorized fallback, got %v", result)
	}
}

func TestResolveWithContext_HitCountTracking(t *testing.T) {
	ruleID := testUUID(10)
	catA := testUUID(1)

	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:         ruleID,
				categoryID: catA,
				condition:  mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "coffee"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	// Two matching transactions.
	tctx := TransactionContext{Name: "Coffee Shop", Provider: "plaid"}
	r.ResolveWithContext("plaid", tctx)
	r.ResolveWithContext("plaid", tctx)

	if r.hitCounts[ruleID.Bytes] != 2 {
		t.Errorf("expected 2 hits, got %d", r.hitCounts[ruleID.Bytes])
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

	ruleAID := formatUUID(ruleA)
	ruleBID := formatUUID(ruleB)

	if parsed[ruleAID] != 5 {
		t.Errorf("expected 5 hits for rule A, got %d", parsed[ruleAID])
	}
	if parsed[ruleBID] != 3 {
		t.Errorf("expected 3 hits for rule B, got %d", parsed[ruleBID])
	}
}

func TestHitCountsJSON_IntegrationWithResolveWithContext(t *testing.T) {
	ruleID := testUUID(10)
	catA := testUUID(1)

	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id:         ruleID,
				categoryID: catA,
				condition:  mustCompile(t, &Condition{Field: "name", Op: "contains", Value: "coffee"}),
			},
		},
		uncategorizedID: testUUID(99),
	}

	// Trigger hits.
	tctx := TransactionContext{Name: "Coffee Shop", Provider: "plaid"}
	r.ResolveWithContext("plaid", tctx)
	r.ResolveWithContext("plaid", tctx)
	r.ResolveWithContext("plaid", tctx)

	result := r.HitCountsJSON()
	if result == nil {
		t.Fatal("expected non-nil JSON after resolving")
	}

	var parsed map[string]int
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	ruleIDStr := formatUUID(ruleID)
	if parsed[ruleIDStr] != 3 {
		t.Errorf("expected 3 hits, got %d", parsed[ruleIDStr])
	}
}
