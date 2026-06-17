//go:build !lite

package sync

import "testing"

// --- Metadata conditions -----------------------------------------------------

func TestEvaluateMetadata_Conditions(t *testing.T) {
	meta := map[string]any{
		"tax_deductible": true,
		"trip":           "japan-2026",
		"amount_cents":   float64(12050), // numbers arrive as float64 from JSON
		"notes":          "extended warranty included",
		"flag_str":       "true",
	}
	cases := []struct {
		name  string
		field string
		op    string
		value any
		want  bool
	}{
		{"bool eq true", "metadata.tax_deductible", "eq", true, true},
		{"bool eq false (mismatch)", "metadata.tax_deductible", "eq", false, false},
		{"bool neq", "metadata.tax_deductible", "neq", false, true},
		{"string-bool eq true", "metadata.flag_str", "eq", true, true},
		{"string eq", "metadata.trip", "eq", "japan-2026", true},
		{"string eq case-insensitive", "metadata.trip", "eq", "JAPAN-2026", true},
		{"string neq", "metadata.trip", "neq", "paris", true},
		{"string contains", "metadata.notes", "contains", "warranty", true},
		{"string not_contains", "metadata.notes", "not_contains", "refund", true},
		{"string matches", "metadata.trip", "matches", "^japan-\\d+$", true},
		{"string in", "metadata.trip", "in", []any{"paris", "japan-2026"}, true},
		{"string in (miss)", "metadata.trip", "in", []any{"paris", "rome"}, false},
		{"numeric gt", "metadata.amount_cents", "gt", float64(10000), true},
		{"numeric gte exact", "metadata.amount_cents", "gte", float64(12050), true},
		{"numeric lt (miss)", "metadata.amount_cents", "lt", float64(100), false},
		{"numeric eq", "metadata.amount_cents", "eq", float64(12050), true},
		{"exists present", "metadata.trip", "exists", nil, true},
		{"not_exists present (miss)", "metadata.trip", "not_exists", nil, false},
		// Absent key: only not_exists matches; every comparison is false.
		{"absent exists (miss)", "metadata.missing", "exists", nil, false},
		{"absent not_exists", "metadata.missing", "not_exists", nil, true},
		{"absent eq (miss)", "metadata.missing", "eq", "x", false},
		{"absent neq (miss — key must be present)", "metadata.missing", "neq", "x", false},
		{"absent not_contains (miss — key must be present)", "metadata.missing", "not_contains", "x", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc := mustCompile(t, &Condition{Field: tc.field, Op: tc.op, Value: tc.value})
			got := evaluateCondition(cc, TransactionContext{Metadata: meta})
			if got != tc.want {
				t.Errorf("metadata cond %s %s %v: got %v, want %v", tc.field, tc.op, tc.value, got, tc.want)
			}
		})
	}
}

func TestEvaluateMetadata_NilBlob(t *testing.T) {
	cc := mustCompile(t, &Condition{Field: "metadata.foo", Op: "eq", Value: "bar"})
	if evaluateCondition(cc, TransactionContext{}) {
		t.Error("eq against a nil metadata blob must not match")
	}
	ccNotExists := mustCompile(t, &Condition{Field: "metadata.foo", Op: "not_exists"})
	if !evaluateCondition(ccNotExists, TransactionContext{}) {
		t.Error("not_exists against a nil metadata blob must match")
	}
}

// --- Metadata actions (resolve merge) ---------------------------------------

func TestResolveWithContext_SetMetadata(t *testing.T) {
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{{
			id:      testUUID(1),
			shortID: "rmeta001",
			name:    "tag deductible",
			actions: []typedAction{
				{Type: "set_metadata", MetadataKey: "tax_deductible", MetadataValue: true},
				{Type: "set_metadata", MetadataKey: "reviewed_by", MetadataValue: "rules"},
			},
			trigger:   "always",
			condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "irs"}),
		}},
	}
	result := r.ResolveWithContext("plaid", TransactionContext{Name: "IRS payment"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if v, ok := result.MetadataSet["tax_deductible"]; !ok || v != true {
		t.Errorf("expected tax_deductible=true, got %v (ok=%v)", v, ok)
	}
	if v, ok := result.MetadataSet["reviewed_by"]; !ok || v != "rules" {
		t.Errorf("expected reviewed_by=rules, got %v (ok=%v)", v, ok)
	}
}

func TestResolveWithContext_SetMetadataLastWriterWins(t *testing.T) {
	// Two rules set the same key; the later (higher-priority-stage) rule wins.
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id: testUUID(1), shortID: "a", name: "A",
				actions:   []typedAction{{Type: "set_metadata", MetadataKey: "k", MetadataValue: "first"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "x"}),
			},
			{
				id: testUUID(2), shortID: "b", name: "B",
				actions:   []typedAction{{Type: "set_metadata", MetadataKey: "k", MetadataValue: "second"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "x"}),
			},
		},
	}
	result := r.ResolveWithContext("plaid", TransactionContext{Name: "x thing"}, true)
	if result.MetadataSet["k"] != "second" {
		t.Errorf("expected last-writer-wins value 'second', got %v", result.MetadataSet["k"])
	}
	// Only the winning rule's source survives for this key.
	metaSources := 0
	for _, s := range result.Sources {
		if s.ActionField == "metadata" && s.ActionValue == "k" {
			metaSources++
		}
	}
	if metaSources != 1 {
		t.Errorf("expected 1 surviving metadata source for key k, got %d", metaSources)
	}
}

func TestResolveWithContext_MetadataNetDiff_SetThenRemoveCancels(t *testing.T) {
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id: testUUID(1), shortID: "a", name: "A",
				actions:   []typedAction{{Type: "set_metadata", MetadataKey: "k", MetadataValue: "v"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "x"}),
			},
			{
				id: testUUID(2), shortID: "b", name: "B",
				actions:   []typedAction{{Type: "remove_metadata", MetadataKey: "k"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "x"}),
			},
		},
	}
	result := r.ResolveWithContext("plaid", TransactionContext{Name: "x thing"}, true)
	if _, ok := result.MetadataSet["k"]; ok {
		t.Errorf("set then remove of same key should cancel; MetadataSet still has k: %v", result.MetadataSet)
	}
	if indexExact(result.MetadataRemove, "k") >= 0 {
		t.Errorf("cancelled set/remove should not queue a delete; MetadataRemove=%v", result.MetadataRemove)
	}
}

// A set followed by a remove of a key that ALREADY exists in the DB blob must
// still delete it — last-writer-wins. Cancelling only the queued set would
// leave the pre-existing value in place, silently dropping the remove.
func TestResolveWithContext_MetadataNetDiff_SetThenRemoveDeletesPreexisting(t *testing.T) {
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id: testUUID(1), shortID: "a", name: "A",
				actions:   []typedAction{{Type: "set_metadata", MetadataKey: "k", MetadataValue: "v"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "x"}),
			},
			{
				id: testUUID(2), shortID: "b", name: "B",
				actions:   []typedAction{{Type: "remove_metadata", MetadataKey: "k"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "x"}),
			},
		},
	}
	// Seed an existing value: set-then-remove must delete it, not revert to it.
	result := r.ResolveWithContext("plaid", TransactionContext{Name: "x thing", Metadata: map[string]any{"k": "old"}}, true)
	if _, ok := result.MetadataSet["k"]; ok {
		t.Errorf("set then remove should not persist a set; MetadataSet=%v", result.MetadataSet)
	}
	if indexExact(result.MetadataRemove, "k") < 0 {
		t.Errorf("set then remove of a pre-existing key must queue a delete; MetadataRemove=%v", result.MetadataRemove)
	}
}

func TestResolveWithContext_MetadataNetDiff_RemoveThenSetWins(t *testing.T) {
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id: testUUID(1), shortID: "a", name: "A",
				actions:   []typedAction{{Type: "remove_metadata", MetadataKey: "k"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "x"}),
			},
			{
				id: testUUID(2), shortID: "b", name: "B",
				actions:   []typedAction{{Type: "set_metadata", MetadataKey: "k", MetadataValue: "v"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "x"}),
			},
		},
	}
	// Seed an existing value so the remove has something to target.
	result := r.ResolveWithContext("plaid", TransactionContext{Name: "x thing", Metadata: map[string]any{"k": "old"}}, true)
	if result.MetadataSet["k"] != "v" {
		t.Errorf("remove-then-set should end as set; got MetadataSet=%v", result.MetadataSet)
	}
	if indexExact(result.MetadataRemove, "k") >= 0 {
		t.Errorf("remove-then-set should not also queue a delete; MetadataRemove=%v", result.MetadataRemove)
	}
}

func TestResolveWithContext_MetadataChaining(t *testing.T) {
	// Rule A writes metadata.bucket; rule B (later) conditions on it.
	r := &RuleResolver{
		hitCounts: make(map[[16]byte]int),
		rules: []compiledRule{
			{
				id: testUUID(1), shortID: "a", name: "A",
				actions:   []typedAction{{Type: "set_metadata", MetadataKey: "bucket", MetadataValue: "subscriptions"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "provider_name", Op: "contains", Value: "spotify"}),
			},
			{
				id: testUUID(2), shortID: "b", name: "B",
				actions:   []typedAction{{Type: "add_tag", TagSlug: "recurring"}},
				trigger:   "always",
				condition: mustCompile(t, &Condition{Field: "metadata.bucket", Op: "eq", Value: "subscriptions"}),
			},
		},
	}
	result := r.ResolveWithContext("plaid", TransactionContext{Name: "Spotify"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.TagsToAdd) != 1 || result.TagsToAdd[0] != "recurring" {
		t.Errorf("later rule should observe rule A's metadata write and add 'recurring'; got %v", result.TagsToAdd)
	}
}
