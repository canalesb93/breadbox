//go:build !headless && !lite

package admin

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestParseAgentDefinitionForm_TriggerOnSyncComplete pins the contract that the
// "Run on sync complete" checkbox round-trips correctly through the create
// form. The form renders `<input type="checkbox" value="true">` so the
// browser submits `trigger_on_sync_complete=true` when checked. Prior to the
// fix the create handler only accepted `=="on"`, silently dropping the
// trigger on every newly created agent.
func TestParseAgentDefinitionForm_TriggerOnSyncComplete(t *testing.T) {
	cases := []struct {
		name string
		form url.Values
		want bool
	}{
		{"checked_true_value", url.Values{
			"name":                     {"Test"},
			"slug":                     {"test"},
			"prompt":                   {"hi"},
			"trigger_on_sync_complete": {"true"},
		}, true},
		{"checked_on_value", url.Values{
			"name":                     {"Test"},
			"slug":                     {"test"},
			"prompt":                   {"hi"},
			"trigger_on_sync_complete": {"on"},
		}, true},
		{"unchecked", url.Values{
			"name":   {"Test"},
			"slug":   {"test"},
			"prompt": {"hi"},
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/-/agents", strings.NewReader(tc.form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			params, err := parseAgentDefinitionForm(r)
			if err != nil {
				t.Fatalf("parseAgentDefinitionForm: %v", err)
			}
			if params.TriggerOnSyncComplete != tc.want {
				t.Errorf("TriggerOnSyncComplete = %v, want %v", params.TriggerOnSyncComplete, tc.want)
			}
		})
	}
}

// TestParseAgentDefinitionForm_Enabled pins the Enabled checkbox too — same
// shape, both `value="true"` and `value="on"` must work.
func TestParseAgentDefinitionForm_Enabled(t *testing.T) {
	cases := []struct {
		name    string
		enabled string
		want    bool
	}{
		{"value_true", "true", true},
		{"value_on", "on", true},
		{"absent", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			form := url.Values{
				"name":   {"Test"},
				"slug":   {"test"},
				"prompt": {"hi"},
			}
			if tc.enabled != "" {
				form.Set("enabled", tc.enabled)
			}
			r := httptest.NewRequest("POST", "/-/agents", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			params, err := parseAgentDefinitionForm(r)
			if err != nil {
				t.Fatalf("parseAgentDefinitionForm: %v", err)
			}
			if params.Enabled != tc.want {
				t.Errorf("Enabled = %v, want %v", params.Enabled, tc.want)
			}
		})
	}
}
