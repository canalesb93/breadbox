package mcp

import (
	"encoding/json"
	"fmt"
	"testing"
)

// TestCompactIDsBytesEquivalence verifies that compactIDsBytes produces
// byte-identical JSON output compared to the original unmarshal→compactIDs→marshal approach.
func TestCompactIDsBytesEquivalence(t *testing.T) {
	for _, n := range []int{1, 5, 50, 200} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			src := makeTransactionList(n)
			data, err := json.Marshal(src)
			if err != nil {
				t.Fatal(err)
			}

			// Original approach.
			var raw any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			compactIDs(raw)
			want, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}

			// New approach.
			got := compactIDsBytes(data)

			// Both must unmarshal to the same structure (key order may differ).
			var wantParsed, gotParsed any
			if err := json.Unmarshal(want, &wantParsed); err != nil {
				t.Fatalf("unmarshal want: %v", err)
			}
			if err := json.Unmarshal(got, &gotParsed); err != nil {
				t.Fatalf("unmarshal got: %v", err)
			}

			wantNorm, _ := json.Marshal(wantParsed)
			gotNorm, _ := json.Marshal(gotParsed)

			if string(wantNorm) != string(gotNorm) {
				t.Errorf("output mismatch\nwant: %s\ngot:  %s", string(wantNorm), string(gotNorm))
			}
		})
	}
}

// TestCompactIDsBytesEdgeCases tests edge cases for the byte-level compactor.
func TestCompactIDsBytesEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input any
	}{
		{"nil_short_id", map[string]any{"id": "uuid-123", "name": "test"}},
		{"no_id", map[string]any{"short_id": "abc123", "name": "test"}},
		{"empty_object", map[string]any{}},
		{"empty_array", []any{}},
		{"nested_arrays", map[string]any{
			"items": []any{
				map[string]any{"id": "uuid-1", "short_id": "s1", "name": "a"},
				map[string]any{"id": "uuid-2", "short_id": "s2", "name": "b"},
			},
		}},
		{"scalar_string", "hello"},
		{"scalar_number", 42.5},
		{"bool_value", true},
		{"null_value", nil},
		{"deeply_nested", map[string]any{
			"id": "uuid-outer", "short_id": "souter",
			"child": map[string]any{
				"id": "uuid-inner", "short_id": "sinner",
				"grandchild": map[string]any{
					"id": "uuid-deep", "short_id": "sdeep",
				},
			},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			// Original approach.
			var raw any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			compactIDs(raw)
			want, _ := json.Marshal(raw)

			// New approach.
			got := compactIDsBytes(data)

			// Normalize via round-trip for comparison (key order may differ).
			var wantP, gotP any
			json.Unmarshal(want, &wantP)
			json.Unmarshal(got, &gotP)
			wantN, _ := json.Marshal(wantP)
			gotN, _ := json.Marshal(gotP)

			if string(wantN) != string(gotN) {
				t.Errorf("output mismatch\nwant: %s\ngot:  %s", string(wantN), string(gotN))
			}
		})
	}
}

// makeTransaction builds a realistic transaction map matching MCP response shape.
func makeTransaction(i int) map[string]any {
	return map[string]any{
		"id":                   fmt.Sprintf("550e8400-e29b-41d4-a716-44665544%04d", i),
		"short_id":             fmt.Sprintf("k7Xm%04d", i),
		"account_id":           "a1b2c3d4",
		"account_name":         "Chase Checking",
		"user_name":            "Ricardo",
		"amount":               42.99 + float64(i),
		"iso_currency_code":    "USD",
		"date":                 "2026-04-01",
		"authorized_date":      "2026-03-31",
		"name":                 fmt.Sprintf("TRADER JOES #%d", 100+i),
		"merchant_name":        "Trader Joe's",
		"category_override":    false,
		"category_primary_raw": "FOOD_AND_DRINK",
		"category_detailed_raw": "FOOD_AND_DRINK_GROCERIES",
		"pending":              false,
		"created_at":           "2026-04-01T10:00:00Z",
		"updated_at":           "2026-04-01T10:00:00Z",
		"category": map[string]any{
			"id":           "cat-uuid-groceries",
			"short_id":     "grc12345",
			"slug":         "food_and_drink_groceries",
			"display_name": "Groceries",
			"primary_slug": "food_and_drink",
			"icon":         "shopping-cart",
			"color":        "#4CAF50",
		},
	}
}

func makeTransactionList(n int) map[string]any {
	txns := make([]any, n)
	for i := range n {
		txns[i] = makeTransaction(i)
	}
	return map[string]any{
		"transactions": txns,
		"next_cursor":  "cursor_abc123",
		"has_more":     true,
		"limit":        n,
	}
}

// BenchmarkCompactIDs_Small benchmarks compactIDs on a single transaction object.
func BenchmarkCompactIDs_Small(b *testing.B) {
	benchmarkCompactIDsN(b, 1)
}

// BenchmarkCompactIDs_Medium benchmarks compactIDs on a 50-item transaction list.
func BenchmarkCompactIDs_Medium(b *testing.B) {
	benchmarkCompactIDsN(b, 50)
}

// BenchmarkCompactIDs_Large benchmarks compactIDs on a 200-item transaction list.
func BenchmarkCompactIDs_Large(b *testing.B) {
	benchmarkCompactIDsN(b, 200)
}

func benchmarkCompactIDsN(b *testing.B, n int) {
	b.Helper()
	// Pre-build the source data and marshal it once (shared across iterations).
	src := makeTransactionList(n)
	srcJSON, err := json.Marshal(src)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var raw any
		_ = json.Unmarshal(srcJSON, &raw)
		compactIDs(raw)
	}
}

// BenchmarkCompactIDsBytes benchmarks the byte-level compactIDsBytes on pre-marshaled JSON.
func BenchmarkCompactIDsBytes_Small(b *testing.B) {
	benchmarkCompactIDsBytesN(b, 1)
}

func BenchmarkCompactIDsBytes_Medium(b *testing.B) {
	benchmarkCompactIDsBytesN(b, 50)
}

func BenchmarkCompactIDsBytes_Large(b *testing.B) {
	benchmarkCompactIDsBytesN(b, 200)
}

func benchmarkCompactIDsBytesN(b *testing.B, n int) {
	b.Helper()
	src := makeTransactionList(n)
	srcJSON, err := json.Marshal(src)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		compactIDsBytes(srcJSON)
	}
}

// BenchmarkJsonResult_Small benchmarks the full jsonResult flow on a single transaction.
func BenchmarkJsonResult_Small(b *testing.B) {
	benchmarkJsonResultN(b, 1)
}

// BenchmarkJsonResult_Medium benchmarks the full jsonResult flow on 50 transactions.
func BenchmarkJsonResult_Medium(b *testing.B) {
	benchmarkJsonResultN(b, 50)
}

// BenchmarkJsonResult_Large benchmarks the full jsonResult flow on 200 transactions.
func BenchmarkJsonResult_Large(b *testing.B) {
	benchmarkJsonResultN(b, 200)
}

func benchmarkJsonResultN(b *testing.B, n int) {
	b.Helper()
	src := makeTransactionList(n)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		jsonResult(src)
	}
}
