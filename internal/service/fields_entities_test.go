//go:build !lite

package service

import (
	"encoding/json"
	"testing"
)

func TestParseSeriesFields(t *testing.T) {
	// Empty → nil (full struct).
	f, err := ParseSeriesFields("")
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if f != nil {
		t.Errorf("empty selection should be nil, got %v", f)
	}

	// overview alias excludes detection_signals; id/short_id always present.
	f, err = ParseSeriesFields("overview")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if f["detection_signals"] {
		t.Error("overview must not include detection_signals")
	}
	for _, want := range []string{"id", "short_id", "name", "status", "next_expected_date"} {
		if !f[want] {
			t.Errorf("overview missing %q", want)
		}
	}

	// Unknown field rejected.
	if _, err := ParseSeriesFields("bogus_series_field"); err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestFilterSeriesFields_DropsDetectionSignals(t *testing.T) {
	s := SeriesResponse{
		ID:               "uuid",
		ShortID:          "abc123",
		Name:             "Netflix",
		Status:           "active",
		DetectionSignals: json.RawMessage(`{"interval_cv":0.02}`),
	}
	f, err := ParseSeriesFields(DefaultSeriesFields)
	if err != nil {
		t.Fatal(err)
	}
	m := FilterSeriesFields(s, f)
	if _, present := m["detection_signals"]; present {
		t.Error("default projection must omit detection_signals")
	}
	if m["name"] != "Netflix" {
		t.Errorf("name = %v, want Netflix", m["name"])
	}
	if m["id"] != "uuid" || m["short_id"] != "abc123" {
		t.Errorf("id/short_id must always be present, got id=%v short_id=%v", m["id"], m["short_id"])
	}

	// fields=all path → nil set → full struct (caller uses struct directly).
	all, err := ParseSeriesFields("")
	if err != nil {
		t.Fatal(err)
	}
	if FilterSeriesFields(s, all) != nil {
		t.Error("nil field set must return nil (full struct)")
	}
}

func TestParseRuleFields(t *testing.T) {
	f, err := ParseRuleFields("summary")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if f["conditions"] || f["actions"] {
		t.Error("summary must not include conditions/actions trees")
	}
	for _, want := range []string{"id", "short_id", "name", "enabled", "priority", "hit_count"} {
		if !f[want] {
			t.Errorf("summary missing %q", want)
		}
	}
}

func TestFilterRuleFields_DropsTrees(t *testing.T) {
	r := TransactionRuleResponse{
		ID:         "uuid",
		ShortID:    "rul456",
		Name:       "Groceries",
		Enabled:    true,
		Priority:   10,
		Conditions: Condition{Field: "provider_name", Op: "contains", Value: "Whole Foods"},
		Actions:    []RuleAction{{Type: "set_category"}},
		HitCount:   42,
	}
	f, err := ParseRuleFields(DefaultRuleFields)
	if err != nil {
		t.Fatal(err)
	}
	m := FilterRuleFields(r, f)
	if _, present := m["conditions"]; present {
		t.Error("default projection must omit conditions")
	}
	if _, present := m["actions"]; present {
		t.Error("default projection must omit actions")
	}
	if m["name"] != "Groceries" || m["hit_count"] != 42 {
		t.Errorf("summary fields wrong: name=%v hit_count=%v", m["name"], m["hit_count"])
	}

	// Explicit conditions request includes the tree.
	withCond, err := ParseRuleFields("summary,conditions,actions")
	if err != nil {
		t.Fatal(err)
	}
	m2 := FilterRuleFields(r, withCond)
	if _, present := m2["conditions"]; !present {
		t.Error("explicit conditions request must include the tree")
	}
	if _, present := m2["actions"]; !present {
		t.Error("explicit actions request must include the tree")
	}
}
