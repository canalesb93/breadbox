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
			name:    "empty condition",
			cond:    Condition{},
			wantErr: true,
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
