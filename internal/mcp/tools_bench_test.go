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
		"provider_name":              fmt.Sprintf("TRADER JOES #%d", 100+i),
		"provider_merchant_name":     "Trader Joe's",
		"category_override":          false,
		"provider_category_primary":  "FOOD_AND_DRINK",
		"provider_category_detailed": "FOOD_AND_DRINK_GROCERIES",
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
	// Benchmark the full old code path as it existed in jsonResult:
	//   json.Marshal(v) → json.Unmarshal → compactIDs → json.Marshal
	// All four steps must be measured for an honest baseline.
	src := makeTransactionList(n)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		data, _ := json.Marshal(src)
		var raw any
		_ = json.Unmarshal(data, &raw)
		compactIDs(raw)
		_, _ = json.Marshal(raw)
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

// TestCompactIDsBytesFieldOrdering verifies that compaction works regardless
// of whether "short_id" appears before or after "id" in the JSON byte stream.
// The two-phase object scanner collects all entries before emitting, so
// ordering does not matter.
func TestCompactIDsBytesFieldOrdering(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"id_before_short_id", `{"id":"uuid-123","short_id":"abc","name":"test"}`},
		{"short_id_before_id", `{"short_id":"abc","id":"uuid-123","name":"test"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := compactIDsBytes([]byte(tc.json))
			var parsed map[string]any
			if err := json.Unmarshal(got, &parsed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if parsed["id"] != "abc" {
				t.Errorf("expected id='abc', got id=%q", parsed["id"])
			}
			if _, hasShort := parsed["short_id"]; hasShort {
				t.Error("short_id should be removed")
			}
		})
	}

	// Struct with ShortID declared before ID — compaction still applies.
	type ReversedDeclOrder struct {
		ShortID string `json:"short_id"`
		ID      string `json:"id"`
		Name    string `json:"name"`
	}
	s := ReversedDeclOrder{ShortID: "compact1", ID: "full-uuid", Name: "test"}
	data, _ := json.Marshal(s)
	got := compactIDsBytes(data)
	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal struct: %v", err)
	}
	if parsed["id"] != "compact1" {
		t.Errorf("struct: expected id='compact1', got id=%q", parsed["id"])
	}
	if _, hasShort := parsed["short_id"]; hasShort {
		t.Error("struct: short_id should be removed")
	}
}

// TestCompactIDsBytesFKPairs verifies that FK sibling pairs like
// account_id/account_short_id are collapsed to a single account_id carrying
// the short value.
func TestCompactIDsBytesFKPairs(t *testing.T) {
	input := []byte(`{"id":"own-uuid","short_id":"ownshort","account_id":"acct-uuid","account_short_id":"acctshort","category_id":"cat-uuid","category_short_id":"catshort","name":"t1"}`)
	got := compactIDsBytes(input)
	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := map[string]string{
		"id":          "ownshort",
		"account_id":  "acctshort",
		"category_id": "catshort",
		"name":        "t1",
	}
	for k, v := range want {
		if parsed[k] != v {
			t.Errorf("%s: got %v want %q", k, parsed[k], v)
		}
	}
	for _, k := range []string{"short_id", "account_short_id", "category_short_id"} {
		if _, has := parsed[k]; has {
			t.Errorf("%s should be removed", k)
		}
	}
}

// TestCompactIDsBytesFKNullShort verifies that a null short_id sibling drops
// the _short_id key but leaves the _id value untouched (compaction would
// otherwise discard a valid UUID).
func TestCompactIDsBytesFKNullShort(t *testing.T) {
	input := []byte(`{"id":"own-uuid","short_id":"ownshort","account_id":"acct-uuid","account_short_id":null}`)
	got := compactIDsBytes(input)
	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["id"] != "ownshort" {
		t.Errorf("id: got %v", parsed["id"])
	}
	if parsed["account_id"] != "acct-uuid" {
		t.Errorf("account_id: expected preserved UUID, got %v", parsed["account_id"])
	}
	if _, has := parsed["account_short_id"]; has {
		t.Error("account_short_id should be removed even when null")
	}
}
