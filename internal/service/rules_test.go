package service

import (
	"testing"
)

func TestValidateCondition_Simple(t *testing.T) {
	tests := []struct {
		name    string
		cond    Condition
		wantErr bool
	}{
		{
			name:    "valid string contains",
			cond:    Condition{Field: "name", Op: "contains", Value: "uber"},
			wantErr: false,
		},
		{
			name:    "valid string eq",
			cond:    Condition{Field: "merchant_name", Op: "eq", Value: "Starbucks"},
			wantErr: false,
		},
		{
			name:    "valid numeric gte",
			cond:    Condition{Field: "amount", Op: "gte", Value: float64(20)},
			wantErr: false,
		},
		{
			name:    "valid bool eq",
			cond:    Condition{Field: "pending", Op: "eq", Value: true},
			wantErr: false,
		},
		{
			name:    "valid regex",
			cond:    Condition{Field: "name", Op: "matches", Value: "(?i)uber.*eats"},
			wantErr: false,
		},
		{
			name:    "valid in operator",
			cond:    Condition{Field: "provider", Op: "in", Value: []interface{}{"plaid", "teller"}},
			wantErr: false,
		},
		{
			name:    "unknown field",
			cond:    Condition{Field: "unknown", Op: "eq", Value: "test"},
			wantErr: true,
		},
		{
			name:    "invalid operator for string",
			cond:    Condition{Field: "name", Op: "gt", Value: "test"},
			wantErr: true,
		},
		{
			name:    "invalid operator for numeric",
			cond:    Condition{Field: "amount", Op: "contains", Value: float64(10)},
			wantErr: true,
		},
		{
			name:    "invalid operator for bool",
			cond:    Condition{Field: "pending", Op: "contains", Value: true},
			wantErr: true,
		},
		{
			name:    "invalid regex",
			cond:    Condition{Field: "name", Op: "matches", Value: "[invalid"},
			wantErr: true,
		},
		{
			name:    "numeric value for string field",
			cond:    Condition{Field: "amount", Op: "eq", Value: "not a number"},
			wantErr: true,
		},
		{
			// An empty Condition{} means "match every transaction" and is
			// stored as NULL in the DB. Rules with no conditions fire on every
			// transaction matching the trigger.
			name:    "empty condition",
			cond:    Condition{},
			wantErr: false,
		},
		{
			name:    "missing operator",
			cond:    Condition{Field: "name", Value: "test"},
			wantErr: true,
		},
		{
			name:    "in operator with empty array",
			cond:    Condition{Field: "name", Op: "in", Value: []interface{}{}},
			wantErr: true,
		},
		{
			name:    "in operator with empty string array",
			cond:    Condition{Field: "provider", Op: "in", Value: []string{}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCondition(tt.cond)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCondition() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCondition_Logical(t *testing.T) {
	t.Run("valid AND", func(t *testing.T) {
		cond := Condition{
			And: []Condition{
				{Field: "name", Op: "contains", Value: "uber"},
				{Field: "amount", Op: "gte", Value: float64(20)},
			},
		}
		if err := ValidateCondition(cond); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid OR", func(t *testing.T) {
		cond := Condition{
			Or: []Condition{
				{Field: "name", Op: "contains", Value: "uber"},
				{Field: "name", Op: "contains", Value: "lyft"},
			},
		}
		if err := ValidateCondition(cond); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid NOT", func(t *testing.T) {
		cond := Condition{
			Not: &Condition{Field: "pending", Op: "eq", Value: true},
		}
		if err := ValidateCondition(cond); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("mixed field and logical rejected", func(t *testing.T) {
		cond := Condition{
			Field: "name",
			Op:    "eq",
			Value: "test",
			And: []Condition{
				{Field: "amount", Op: "gt", Value: float64(10)},
			},
		}
		if err := ValidateCondition(cond); err == nil {
			t.Error("expected error for mixed field+logical condition")
		}
	})

	t.Run("invalid child in AND", func(t *testing.T) {
		cond := Condition{
			And: []Condition{
				{Field: "name", Op: "contains", Value: "uber"},
				{Field: "unknown", Op: "eq", Value: "test"},
			},
		}
		if err := ValidateCondition(cond); err == nil {
			t.Error("expected error for invalid child")
		}
	})
}

func TestEvaluateCondition(t *testing.T) {
	tctx := TransactionContext{
		Name:             "UBER EATS - ORDER #1234",
		MerchantName:     "Uber Eats",
		Amount:           25.50,
		CategoryPrimary:  "dining",
		CategoryDetailed: "restaurant",
		Pending:          false,
		Provider:         "teller",
		AccountID:        "acc-123",
		UserID:           "user-456",
	}

	tests := []struct {
		name     string
		cond     Condition
		expected bool
	}{
		{
			name:     "string contains match",
			cond:     Condition{Field: "name", Op: "contains", Value: "uber eats"},
			expected: true,
		},
		{
			name:     "string contains no match",
			cond:     Condition{Field: "name", Op: "contains", Value: "doordash"},
			expected: false,
		},
		{
			name:     "string eq case insensitive",
			cond:     Condition{Field: "merchant_name", Op: "eq", Value: "uber eats"},
			expected: true,
		},
		{
			name:     "string neq",
			cond:     Condition{Field: "provider", Op: "neq", Value: "plaid"},
			expected: true,
		},
		{
			name:     "string not_contains",
			cond:     Condition{Field: "name", Op: "not_contains", Value: "doordash"},
			expected: true,
		},
		{
			name:     "numeric gte match",
			cond:     Condition{Field: "amount", Op: "gte", Value: float64(20)},
			expected: true,
		},
		{
			name:     "numeric lt no match",
			cond:     Condition{Field: "amount", Op: "lt", Value: float64(20)},
			expected: false,
		},
		{
			name:     "numeric eq",
			cond:     Condition{Field: "amount", Op: "eq", Value: 25.50},
			expected: true,
		},
		{
			name:     "bool eq match",
			cond:     Condition{Field: "pending", Op: "eq", Value: false},
			expected: true,
		},
		{
			name:     "bool neq",
			cond:     Condition{Field: "pending", Op: "neq", Value: true},
			expected: true,
		},
		{
			name: "AND all match",
			cond: Condition{
				And: []Condition{
					{Field: "name", Op: "contains", Value: "uber"},
					{Field: "amount", Op: "gte", Value: float64(20)},
				},
			},
			expected: true,
		},
		{
			name: "AND partial match",
			cond: Condition{
				And: []Condition{
					{Field: "name", Op: "contains", Value: "uber"},
					{Field: "amount", Op: "gt", Value: float64(100)},
				},
			},
			expected: false,
		},
		{
			name: "OR any match",
			cond: Condition{
				Or: []Condition{
					{Field: "name", Op: "contains", Value: "doordash"},
					{Field: "name", Op: "contains", Value: "uber"},
				},
			},
			expected: true,
		},
		{
			name: "OR no match",
			cond: Condition{
				Or: []Condition{
					{Field: "name", Op: "contains", Value: "doordash"},
					{Field: "name", Op: "contains", Value: "grubhub"},
				},
			},
			expected: false,
		},
		{
			name: "NOT negation",
			cond: Condition{
				Not: &Condition{Field: "pending", Op: "eq", Value: true},
			},
			expected: true,
		},
		{
			name:     "in operator match",
			cond:     Condition{Field: "provider", Op: "in", Value: []interface{}{"plaid", "teller"}},
			expected: true,
		},
		{
			name:     "in operator no match",
			cond:     Condition{Field: "provider", Op: "in", Value: []interface{}{"plaid", "csv"}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc, err := CompileCondition(tt.cond)
			if err != nil {
				t.Fatalf("CompileCondition() error = %v", err)
			}
			result := EvaluateCondition(cc, tctx)
			if result != tt.expected {
				t.Errorf("EvaluateCondition() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEvaluateCondition_Regex(t *testing.T) {
	tctx := TransactionContext{
		Name: "UBER EATS - ORDER #1234",
	}

	cond := Condition{Field: "name", Op: "matches", Value: "(?i)uber.*eats"}
	cc, err := CompileCondition(cond)
	if err != nil {
		t.Fatalf("CompileCondition() error = %v", err)
	}

	if !EvaluateCondition(cc, tctx) {
		t.Error("expected regex to match")
	}

	cond2 := Condition{Field: "name", Op: "matches", Value: "doordash.*"}
	cc2, err := CompileCondition(cond2)
	if err != nil {
		t.Fatalf("CompileCondition() error = %v", err)
	}

	if EvaluateCondition(cc2, tctx) {
		t.Error("expected regex not to match")
	}
}

func TestConditionSummary(t *testing.T) {
	tests := []struct {
		name     string
		cond     Condition
		expected string
	}{
		{
			name:     "match-all empty condition",
			cond:     Condition{},
			expected: "All transactions",
		},
		{
			name:     "match-all empty And slice",
			cond:     Condition{And: []Condition{}},
			expected: "All transactions",
		},
		{
			name:     "simple contains",
			cond:     Condition{Field: "name", Op: "contains", Value: "uber"},
			expected: `name contains "uber"`,
		},
		{
			name: "AND",
			cond: Condition{
				And: []Condition{
					{Field: "name", Op: "contains", Value: "uber"},
					{Field: "amount", Op: "gte", Value: float64(20)},
				},
			},
			expected: `name contains "uber" AND amount >= 20`,
		},
		{
			name: "OR",
			cond: Condition{
				Or: []Condition{
					{Field: "name", Op: "contains", Value: "uber"},
					{Field: "name", Op: "contains", Value: "lyft"},
				},
			},
			expected: `(name contains "uber" OR name contains "lyft")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConditionSummary(tt.cond)
			if result != tt.expected {
				t.Errorf("ConditionSummary() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestActionsSummary(t *testing.T) {
	tests := []struct {
		name     string
		actions  []RuleAction
		catName  string
		expected string
	}{
		{
			name:     "empty",
			actions:  nil,
			expected: "(no actions)",
		},
		{
			name:     "single set_category with display name",
			actions:  []RuleAction{{Type: "set_category", CategorySlug: "food_and_drink_groceries"}},
			catName:  "Groceries",
			expected: "Set category: Groceries",
		},
		{
			name:     "single set_category falls back to slug",
			actions:  []RuleAction{{Type: "set_category", CategorySlug: "food_and_drink_groceries"}},
			expected: "Set category: food_and_drink_groceries",
		},
		{
			name:     "single add_tag",
			actions:  []RuleAction{{Type: "add_tag", TagSlug: "needs-review"}},
			expected: "Add tag needs-review",
		},
		{
			name:     "single add_comment",
			actions:  []RuleAction{{Type: "add_comment", Content: "investigate"}},
			expected: "Add comment",
		},
		{
			name: "multiple actions returns count",
			actions: []RuleAction{
				{Type: "set_category", CategorySlug: "x"},
				{Type: "add_tag", TagSlug: "y"},
			},
			expected: "2 actions",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ActionsSummary(tt.actions, tt.catName)
			if got != tt.expected {
				t.Errorf("ActionsSummary() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestTriggerLabel(t *testing.T) {
	tests := map[string]string{
		"":          "On sync create",
		"on_create": "On sync create",
		"on_change": "On sync change",
		"on_update": "On sync change", // legacy alias
		"always":    "Every sync",
		"weird":     "weird",
	}
	for in, want := range tests {
		if got := TriggerLabel(in); got != want {
			t.Errorf("TriggerLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeTrigger(t *testing.T) {
	cases := []struct {
		in     string
		out    string
		errful bool
	}{
		{"", "on_create", false},
		{"on_create", "on_create", false},
		{"on_change", "on_change", false},
		{"on_update", "on_change", false}, // legacy alias normalized
		{"always", "always", false},
		{"nonsense", "", true},
	}
	for _, tc := range cases {
		got, err := normalizeTrigger(tc.in)
		if tc.errful {
			if err == nil {
				t.Errorf("normalizeTrigger(%q) expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeTrigger(%q) unexpected err: %v", tc.in, err)
		}
		if got != tc.out {
			t.Errorf("normalizeTrigger(%q) = %q want %q", tc.in, got, tc.out)
		}
	}
}

// --- Baseline regression tests (audit §D) ---
//
// These lock down current validation and evaluation semantics so upcoming
// resolver-chaining / priority-inversion work can't silently regress them.

func TestValidateCondition_DepthLimit(t *testing.T) {
	// Build a linear chain of NOT-wrapped conditions. Depth 10 = accepted,
	// depth 11 = rejected (ValidateCondition enforces depth > 10 → error).
	build := func(n int) Condition {
		leaf := Condition{Field: "name", Op: "eq", Value: "x"}
		cur := leaf
		for i := 0; i < n; i++ {
			next := cur
			cur = Condition{Not: &next}
		}
		return cur
	}

	if err := ValidateCondition(build(10)); err != nil {
		t.Errorf("depth 10 should be accepted, got %v", err)
	}
	if err := ValidateCondition(build(11)); err == nil {
		t.Error("depth 11 should be rejected")
	}
}

func TestValidateCondition_TagsField(t *testing.T) {
	cases := []struct {
		name    string
		cond    Condition
		wantErr bool
	}{
		{"contains with string", Condition{Field: "tags", Op: "contains", Value: "needs-review"}, false},
		{"not_contains with string", Condition{Field: "tags", Op: "not_contains", Value: "needs-review"}, false},
		{"in with array", Condition{Field: "tags", Op: "in", Value: []interface{}{"a", "b"}}, false},
		{"eq rejected", Condition{Field: "tags", Op: "eq", Value: "x"}, true},
		{"matches rejected", Condition{Field: "tags", Op: "matches", Value: ".*"}, true},
		{"contains with non-string rejected", Condition{Field: "tags", Op: "contains", Value: 123}, true},
		{"in with empty array rejected", Condition{Field: "tags", Op: "in", Value: []interface{}{}}, true},
		{"in with non-array rejected", Condition{Field: "tags", Op: "in", Value: "notanarray"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCondition(tc.cond)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateCondition() err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestEvaluateCondition_TagsField(t *testing.T) {
	tctx := TransactionContext{Tags: []string{"coffee", "needs-review"}}

	cases := []struct {
		name     string
		cond     Condition
		expected bool
	}{
		{"contains present", Condition{Field: "tags", Op: "contains", Value: "coffee"}, true},
		{"contains absent", Condition{Field: "tags", Op: "contains", Value: "travel"}, false},
		{"contains case-insensitive", Condition{Field: "tags", Op: "contains", Value: "COFFEE"}, true},
		{"not_contains absent", Condition{Field: "tags", Op: "not_contains", Value: "travel"}, true},
		{"not_contains present", Condition{Field: "tags", Op: "not_contains", Value: "coffee"}, false},
		{"in any present", Condition{Field: "tags", Op: "in", Value: []interface{}{"travel", "coffee"}}, true},
		{"in none present", Condition{Field: "tags", Op: "in", Value: []interface{}{"travel", "flagged"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc, err := CompileCondition(tc.cond)
			if err != nil {
				t.Fatalf("CompileCondition err: %v", err)
			}
			if got := EvaluateCondition(cc, tctx); got != tc.expected {
				t.Errorf("EvaluateCondition() = %v want %v", got, tc.expected)
			}
		})
	}
}

func TestValidateCondition_NewFields(t *testing.T) {
	cases := []struct {
		name    string
		cond    Condition
		wantErr bool
	}{
		{"category contains", Condition{Field: "category", Op: "contains", Value: "coffee"}, false},
		{"category eq", Condition{Field: "category", Op: "eq", Value: "food_and_drink_coffee"}, false},
		{"account_name contains", Condition{Field: "account_name", Op: "contains", Value: "chase"}, false},
		{"account_name in", Condition{Field: "account_name", Op: "in", Value: []interface{}{"Checking", "Savings"}}, false},
		{"category numeric op rejected", Condition{Field: "category", Op: "gt", Value: "x"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCondition(tc.cond)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateCondition err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestEvaluateCondition_CategoryAndAccountName(t *testing.T) {
	tctx := TransactionContext{
		Category:    "food_and_drink_coffee",
		AccountName: "Chase Freedom",
	}

	cc := mustCompileSvc(t, Condition{Field: "category", Op: "eq", Value: "food_and_drink_coffee"})
	if !EvaluateCondition(cc, tctx) {
		t.Error("expected category eq to match")
	}

	cc = mustCompileSvc(t, Condition{Field: "category", Op: "contains", Value: "coffee"})
	if !EvaluateCondition(cc, tctx) {
		t.Error("expected category contains to match")
	}

	cc = mustCompileSvc(t, Condition{Field: "account_name", Op: "contains", Value: "chase"})
	if !EvaluateCondition(cc, tctx) {
		t.Error("expected account_name contains to match (case-insensitive)")
	}

	// Empty fields shouldn't match contains of a non-empty value.
	emptyCtx := TransactionContext{}
	if EvaluateCondition(cc, emptyCtx) {
		t.Error("expected account_name contains to fail on empty AccountName")
	}
}

func TestEvaluateCondition_TagsField_EmptyTransactionTags(t *testing.T) {
	tctx := TransactionContext{Tags: nil}

	// contains false on nil tags.
	cc := mustCompileSvc(t, Condition{Field: "tags", Op: "contains", Value: "x"})
	if EvaluateCondition(cc, tctx) {
		t.Error("expected contains on empty tags to be false")
	}

	// not_contains true on nil tags.
	cc = mustCompileSvc(t, Condition{Field: "tags", Op: "not_contains", Value: "x"})
	if !EvaluateCondition(cc, tctx) {
		t.Error("expected not_contains on empty tags to be true")
	}
}

func mustCompileSvc(t *testing.T, c Condition) *CompiledCondition {
	t.Helper()
	cc, err := CompileCondition(c)
	if err != nil {
		t.Fatalf("CompileCondition err: %v", err)
	}
	return cc
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"24h", false},
		{"30d", false},
		{"1w", false},
		{"7d", false},
		{"abc", true},
		{"", true},
		{"30x", true},
		{"-5d", true},
		{"-1h", true},
		{"0d", true},
		{"0h", true},
		{"-0w", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestResolveRulePriority(t *testing.T) {
	intPtr := func(i int) *int { return &i }

	tests := []struct {
		name     string
		stage    string
		priority *int
		want     int
		wantErr  bool
	}{
		// Neither supplied -> default standard (10).
		{name: "empty stage, nil priority defaults to standard", stage: "", priority: nil, want: 10},

		// Stage names map to canonical priorities.
		{name: "baseline maps to 0", stage: "baseline", priority: nil, want: 0},
		{name: "standard maps to 10", stage: "standard", priority: nil, want: 10},
		{name: "refinement maps to 50", stage: "refinement", priority: nil, want: 50},
		{name: "override maps to 100", stage: "override", priority: nil, want: 100},

		// Case-insensitive + trimmed.
		{name: "uppercase stage accepted", stage: "OVERRIDE", priority: nil, want: 100},
		{name: "mixed-case stage accepted", stage: "Refinement", priority: nil, want: 50},
		{name: "stage with whitespace trimmed", stage: "  standard  ", priority: nil, want: 10},

		// Priority wins on conflict. Note: stage is still validated —
		// both-supplied callers must pass a valid stage if any stage is
		// passed. To pass an arbitrary priority, leave stage empty.
		{name: "explicit priority wins over stage", stage: "override", priority: intPtr(7), want: 7},
		{name: "explicit priority of 0 respected", stage: "override", priority: intPtr(0), want: 0},
		{name: "explicit priority with empty stage", stage: "", priority: intPtr(42), want: 42},
		{name: "priority above canonical range accepted", stage: "", priority: intPtr(500), want: 500},

		// Unknown stage -> validation error.
		{name: "unknown stage rejected", stage: "bogus", priority: nil, want: 0, wantErr: true},
		{name: "empty-string stage with spaces is treated as empty", stage: "   ", priority: nil, want: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveRulePriority(tt.stage, tt.priority)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveRulePriority(%q, %v) error = %v, wantErr %v", tt.stage, tt.priority, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("ResolveRulePriority(%q, %v) = %d, want %d", tt.stage, tt.priority, got, tt.want)
			}
		})
	}
}
