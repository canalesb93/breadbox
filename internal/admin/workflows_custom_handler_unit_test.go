//go:build !headless && !lite

package admin

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSlugifyWorkflowName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Daily triage", "daily-triage"},
		{"Weekly  Anomaly   Sweep", "weekly-anomaly-sweep"},
		{"  Leading/trailing!! ", "leading-trailing"},
		{"Spend Report (monthly)", "spend-report-monthly"},
		{"MixedCASE123", "mixedcase123"},
		{"!!!", "workflow"},
		{"", "workflow"},
		{"-- dashes --", "dashes"},
	}
	for _, tc := range cases {
		if got := slugifyWorkflowName(tc.in); got != tc.want {
			t.Errorf("slugifyWorkflowName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSlugifyWorkflowNameLength(t *testing.T) {
	long := strings.Repeat("a", 200)
	got := slugifyWorkflowName(long)
	if len(got) > 56 {
		t.Fatalf("slug too long: %d chars (want <= 56)", len(got))
	}
}

func TestReadCustomWorkflowInput(t *testing.T) {
	parse := func(form url.Values) (customWorkflowInput, error) {
		r := httptest.NewRequest("POST", "/-/custom-workflows", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		return readCustomWorkflowInput(r)
	}

	t.Run("requires name", func(t *testing.T) {
		if _, err := parse(url.Values{"prompt": {"hi"}}); err == nil {
			t.Fatal("expected error for missing name")
		}
	})
	t.Run("requires prompt", func(t *testing.T) {
		if _, err := parse(url.Values{"name": {"X"}}); err == nil {
			t.Fatal("expected error for missing prompt")
		}
	})
	t.Run("defaults tool scope to read_write", func(t *testing.T) {
		in, err := parse(url.Values{"name": {"X"}, "prompt": {"do"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if in.toolScope != "read_write" {
			t.Errorf("toolScope = %q, want read_write", in.toolScope)
		}
	})
	t.Run("read_only preserved", func(t *testing.T) {
		in, _ := parse(url.Values{"name": {"X"}, "prompt": {"do"}, "tool_scope": {"read_only"}})
		if in.toolScope != "read_only" {
			t.Errorf("toolScope = %q, want read_only", in.toolScope)
		}
	})
	t.Run("trigger + schedule + caps parse", func(t *testing.T) {
		in, err := parse(url.Values{
			"name":            {"Sweep"},
			"prompt":          {"do"},
			"trigger_on_sync": {"true"},
			"schedule_cron":   {"0 8 * * *"},
			"max_turns":       {"12"},
			"max_budget_usd":  {"2.50"},
			"enabled":         {"true"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !in.triggerOnSync {
			t.Error("triggerOnSync should be true")
		}
		if in.scheduleCron != "0 8 * * *" {
			t.Errorf("scheduleCron = %q", in.scheduleCron)
		}
		if in.maxTurns != 12 {
			t.Errorf("maxTurns = %d, want 12", in.maxTurns)
		}
		if in.maxBudget == nil || *in.maxBudget != 2.50 {
			t.Errorf("maxBudget = %v, want 2.50", in.maxBudget)
		}
		if !in.enabled {
			t.Error("enabled should be true")
		}
	})
	t.Run("rejects bad budget", func(t *testing.T) {
		if _, err := parse(url.Values{"name": {"X"}, "prompt": {"do"}, "max_budget_usd": {"-1"}}); err == nil {
			t.Fatal("expected error for negative budget")
		}
	})
}
