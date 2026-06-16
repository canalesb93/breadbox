//go:build !lite

package service

import (
	"context"
	"strings"
	"testing"
)

func TestValidateCondition_MetadataField(t *testing.T) {
	cases := []struct {
		name    string
		cond    Condition
		wantErr bool
	}{
		{"eq bool", Condition{Field: "metadata.tax_deductible", Op: "eq", Value: true}, false},
		{"eq string", Condition{Field: "metadata.trip", Op: "eq", Value: "japan"}, false},
		{"gt numeric", Condition{Field: "metadata.cents", Op: "gt", Value: 100}, false},
		{"exists no value", Condition{Field: "metadata.foo", Op: "exists"}, false},
		{"not_exists no value", Condition{Field: "metadata.foo", Op: "not_exists"}, false},
		{"matches valid regex", Condition{Field: "metadata.code", Op: "matches", Value: "^ABC"}, false},
		{"in non-empty array", Condition{Field: "metadata.trip", Op: "in", Value: []interface{}{"a", "b"}}, false},
		{"empty key rejected", Condition{Field: "metadata.", Op: "eq", Value: "x"}, true},
		{"unknown op rejected", Condition{Field: "metadata.k", Op: "weird", Value: "x"}, true},
		{"gt non-numeric rejected", Condition{Field: "metadata.k", Op: "gt", Value: "abc"}, true},
		{"matches non-string rejected", Condition{Field: "metadata.k", Op: "matches", Value: 5}, true},
		{"in empty array rejected", Condition{Field: "metadata.k", Op: "in", Value: []interface{}{}}, true},
		{"oversize key rejected", Condition{Field: "metadata." + strings.Repeat("x", 200), Op: "eq", Value: "y"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateCondition(tc.cond); (err != nil) != tc.wantErr {
				t.Errorf("ValidateCondition err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestEvaluateCondition_MetadataField(t *testing.T) {
	tctx := TransactionContext{Metadata: map[string]any{
		"tax_deductible": true,
		"trip":           "japan-2026",
		"cents":          float64(12050),
		"notes":          "warranty card inside",
	}}
	cases := []struct {
		cond Condition
		want bool
	}{
		{Condition{Field: "metadata.tax_deductible", Op: "eq", Value: true}, true},
		{Condition{Field: "metadata.trip", Op: "eq", Value: "JAPAN-2026"}, true},
		{Condition{Field: "metadata.trip", Op: "contains", Value: "japan"}, true},
		{Condition{Field: "metadata.cents", Op: "gte", Value: 12050}, true},
		{Condition{Field: "metadata.cents", Op: "lt", Value: 100}, false},
		{Condition{Field: "metadata.notes", Op: "matches", Value: "warranty"}, true},
		{Condition{Field: "metadata.foo", Op: "exists"}, false},
		{Condition{Field: "metadata.foo", Op: "not_exists"}, true},
		{Condition{Field: "metadata.foo", Op: "eq", Value: "x"}, false},  // absent → no match
		{Condition{Field: "metadata.foo", Op: "neq", Value: "x"}, false}, // absent → no match (must be present)
	}
	for _, tc := range cases {
		cc := mustCompileSvc(t, tc.cond)
		if got := EvaluateCondition(cc, tctx); got != tc.want {
			t.Errorf("EvaluateCondition(%s %s %v) = %v, want %v", tc.cond.Field, tc.cond.Op, tc.cond.Value, got, tc.want)
		}
	}
}

func TestValidateActions_Metadata(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	cases := []struct {
		name    string
		actions []RuleAction
		wantErr bool
	}{
		{"set_metadata bool", []RuleAction{{Type: "set_metadata", MetadataKey: "k", MetadataValue: true}}, false},
		{"set_metadata object", []RuleAction{{Type: "set_metadata", MetadataKey: "k", MetadataValue: map[string]any{"a": 1}}}, false},
		{"remove_metadata", []RuleAction{{Type: "remove_metadata", MetadataKey: "k"}}, false},
		{"set_metadata empty key", []RuleAction{{Type: "set_metadata", MetadataKey: "", MetadataValue: 1}}, true},
		{"remove_metadata empty key", []RuleAction{{Type: "remove_metadata", MetadataKey: ""}}, true},
		{"set_metadata oversize value", []RuleAction{{Type: "set_metadata", MetadataKey: "k", MetadataValue: strings.Repeat("x", 5000)}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := svc.ValidateActions(ctx, tc.actions); (err != nil) != tc.wantErr {
				t.Errorf("ValidateActions err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
