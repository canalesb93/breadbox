package service

import (
	"fmt"
	"testing"
)

// benchTransactionContext returns a realistic TransactionContext for benchmarking.
func benchTransactionContext() TransactionContext {
	return TransactionContext{
		Name:             "UBER EATS - ORDER #1234",
		MerchantName:     "Uber Eats",
		Amount:           25.50,
		CategoryPrimary:  "dining",
		CategoryDetailed: "restaurant",
		Pending:          false,
		Provider:         "teller",
		AccountID:        "acc-123",
		UserID:           "user-456",
		UserName:         "Alice",
	}
}

// mustCompile compiles a condition and fatals on error.
func mustCompile(b *testing.B, c Condition) *CompiledCondition {
	b.Helper()
	cc, err := CompileCondition(c)
	if err != nil {
		b.Fatalf("CompileCondition: %v", err)
	}
	return cc
}

// BenchmarkEvaluateSimpleContains benchmarks a single "contains" condition on merchant_name.
func BenchmarkEvaluateSimpleContains(b *testing.B) {
	cc := mustCompile(b, Condition{Field: "provider_merchant_name", Op: "contains", Value: "uber"})
	tctx := benchTransactionContext()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EvaluateCondition(cc, tctx)
	}
}

// BenchmarkEvaluateMediumAND benchmarks AND of 3 conditions: contains on name, eq on
// category_primary, gte on amount.
func BenchmarkEvaluateMediumAND(b *testing.B) {
	cc := mustCompile(b, Condition{
		And: []Condition{
			{Field: "provider_name", Op: "contains", Value: "uber eats"},
			{Field: "provider_category_primary", Op: "eq", Value: "dining"},
			{Field: "amount", Op: "gte", Value: float64(20)},
		},
	})
	tctx := benchTransactionContext()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EvaluateCondition(cc, tctx)
	}
}

// BenchmarkEvaluateComplexTree benchmarks a nested AND/OR tree with 5+ conditions including
// "in" and "matches" (regex).
func BenchmarkEvaluateComplexTree(b *testing.B) {
	cc := mustCompile(b, Condition{
		And: []Condition{
			{
				Or: []Condition{
					{Field: "provider_name", Op: "contains", Value: "uber"},
					{Field: "provider_name", Op: "contains", Value: "lyft"},
					{Field: "provider_merchant_name", Op: "matches", Value: "(?i)doordash|grubhub"},
				},
			},
			{Field: "provider", Op: "in", Value: []interface{}{"plaid", "teller", "csv"}},
			{Field: "amount", Op: "gte", Value: float64(10)},
			{
				Not: &Condition{Field: "pending", Op: "eq", Value: true},
			},
			{Field: "provider_category_primary", Op: "neq", Value: "transfer"},
		},
	})
	tctx := benchTransactionContext()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EvaluateCondition(cc, tctx)
	}
}

// BenchmarkEvaluateBatch1000 evaluates one compiled rule against 1000 different
// TransactionContext values, simulating a sync batch.
func BenchmarkEvaluateBatch1000(b *testing.B) {
	cc := mustCompile(b, Condition{
		And: []Condition{
			{Field: "provider_name", Op: "contains", Value: "uber"},
			{Field: "amount", Op: "gte", Value: float64(10)},
			{Field: "provider", Op: "in", Value: []interface{}{"plaid", "teller"}},
		},
	})

	merchants := []string{
		"Uber Eats", "Starbucks", "Whole Foods", "Target", "Costco",
		"Amazon", "Netflix", "Spotify", "Shell", "Walgreens",
	}
	names := []string{
		"UBER EATS ORDER #1234", "STARBUCKS STORE #567", "WHOLEFDS MKT 10432",
		"TARGET 1234", "COSTCO WHSE #789", "AMZN MKTP US*AB1CD2",
		"NETFLIX.COM", "SPOTIFY USA", "SHELL OIL 57442", "WALGREENS #1234",
	}

	tctxs := make([]TransactionContext, 1000)
	for i := range tctxs {
		tctxs[i] = TransactionContext{
			Name:             names[i%len(names)],
			MerchantName:     merchants[i%len(merchants)],
			Amount:           float64(5 + (i % 100)),
			CategoryPrimary:  "dining",
			CategoryDetailed: "restaurant",
			Pending:          i%10 == 0,
			Provider:         []string{"plaid", "teller", "csv"}[i%3],
			AccountID:        fmt.Sprintf("acc-%d", i%5),
			UserID:           fmt.Sprintf("user-%d", i%3),
			UserName:         []string{"Alice", "Bob", "Carol"}[i%3],
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range tctxs {
			EvaluateCondition(cc, tctxs[j])
		}
	}
}

// BenchmarkEvaluateStringEq benchmarks the "eq" string operator in isolation.
func BenchmarkEvaluateStringEq(b *testing.B) {
	cc := mustCompile(b, Condition{Field: "provider_merchant_name", Op: "eq", Value: "Uber Eats"})
	tctx := benchTransactionContext()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EvaluateCondition(cc, tctx)
	}
}

// BenchmarkEvaluateStringIn benchmarks the "in" operator with a 5-element set.
func BenchmarkEvaluateStringIn(b *testing.B) {
	cc := mustCompile(b, Condition{
		Field: "provider",
		Op:    "in",
		Value: []interface{}{"plaid", "teller", "csv", "manual", "other"},
	})
	tctx := benchTransactionContext()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EvaluateCondition(cc, tctx)
	}
}

// BenchmarkCompileCondition benchmarks CompileCondition for the complex tree.
func BenchmarkCompileCondition(b *testing.B) {
	cond := Condition{
		And: []Condition{
			{
				Or: []Condition{
					{Field: "provider_name", Op: "contains", Value: "uber"},
					{Field: "provider_name", Op: "contains", Value: "lyft"},
					{Field: "provider_merchant_name", Op: "matches", Value: "(?i)doordash|grubhub"},
				},
			},
			{Field: "provider", Op: "in", Value: []interface{}{"plaid", "teller", "csv"}},
			{Field: "amount", Op: "gte", Value: float64(10)},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompileCondition(cond)
	}
}
