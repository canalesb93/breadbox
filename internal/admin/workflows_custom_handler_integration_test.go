//go:build integration && !headless && !lite

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// customWorkflowRouter wires the three custom-workflow handlers on a minimal
// chi router with the {slug} param, bypassing RequireAdmin for handler-level
// coverage.
func customWorkflowRouter(svc *service.Service) *chi.Mux {
	r := chi.NewRouter()
	r.Post("/-/custom-workflows", CreateCustomWorkflowAdminHandler(svc))
	r.Get("/-/custom-workflows/{slug}", CustomWorkflowConfigAdminHandler(svc))
	r.Post("/-/custom-workflows/{slug}", UpdateCustomWorkflowAdminHandler(svc))
	return r
}

func postForm(t *testing.T, r *chi.Mux, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestCustomWorkflowCreateConfigUpdate exercises the full custom-workflow
// lifecycle through the admin handlers: create a hand-authored workflow, read
// its config back, edit the prompt + trigger, and confirm the definition
// round-trips with source_template still NULL.
func TestCustomWorkflowCreateConfigUpdate(t *testing.T) {
	svc := newTestSvc(t)
	r := customWorkflowRouter(svc)

	// --- Create ---
	w := postForm(t, r, "/-/custom-workflows", url.Values{
		"name":            {"My Anomaly Sweep"},
		"prompt":          {"Watch for unusual charges and report them."},
		"trigger_on_sync": {"false"},
		"schedule_cron":   {"0 8 * * *"},
		"model":           {"claude-sonnet-4-6"},
		"tool_scope":      {"read_only"},
		"max_budget_usd":  {"3.00"},
		"enabled":         {"true"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create: status %d, body %s", w.Code, w.Body.String())
	}
	var created struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("create: decode: %v", err)
	}
	if created.Slug != "my-anomaly-sweep" {
		t.Fatalf("create: slug = %q, want my-anomaly-sweep", created.Slug)
	}

	// The created definition must be hand-authored (source_template NULL).
	def, err := svc.GetAgentDefinition(t.Context(), created.Slug)
	if err != nil {
		t.Fatalf("GetAgentDefinition: %v", err)
	}
	if def.SourceTemplate != nil {
		t.Fatalf("source_template = %q, want NULL (custom)", *def.SourceTemplate)
	}
	if def.ToolScope != "read_only" {
		t.Errorf("tool_scope = %q, want read_only", def.ToolScope)
	}
	if def.ScheduleCron == nil || *def.ScheduleCron != "0 8 * * *" {
		t.Errorf("schedule_cron = %v, want 0 8 * * *", def.ScheduleCron)
	}

	// --- Config (edit prefill) ---
	cfgReq := httptest.NewRequest(http.MethodGet, "/-/custom-workflows/"+created.Slug, nil)
	cfgW := httptest.NewRecorder()
	r.ServeHTTP(cfgW, cfgReq)
	if cfgW.Code != http.StatusOK {
		t.Fatalf("config: status %d, body %s", cfgW.Code, cfgW.Body.String())
	}
	var cfg struct {
		Name      string `json:"name"`
		Prompt    string `json:"prompt"`
		ToolScope string `json:"tool_scope"`
	}
	if err := json.Unmarshal(cfgW.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("config: decode: %v", err)
	}
	if cfg.Prompt != "Watch for unusual charges and report them." {
		t.Errorf("config prompt mismatch: %q", cfg.Prompt)
	}

	// --- Update: rewrite prompt + switch to post-sync ---
	uw := postForm(t, r, "/-/custom-workflows/"+created.Slug, url.Values{
		"name":            {"My Anomaly Sweep"},
		"prompt":          {"Updated instructions."},
		"trigger_on_sync": {"true"},
		"model":           {"claude-sonnet-4-6"},
		"tool_scope":      {"read_write"},
	})
	if uw.Code != http.StatusOK {
		t.Fatalf("update: status %d, body %s", uw.Code, uw.Body.String())
	}
	def2, err := svc.GetAgentDefinition(t.Context(), created.Slug)
	if err != nil {
		t.Fatalf("GetAgentDefinition (post-update): %v", err)
	}
	if def2.Prompt != "Updated instructions." {
		t.Errorf("updated prompt = %q", def2.Prompt)
	}
	if !def2.TriggerOnSyncComplete {
		t.Error("trigger_on_sync_complete should be true after update")
	}
	if def2.ScheduleCron != nil && *def2.ScheduleCron != "" {
		t.Errorf("schedule_cron should be cleared when switching to post-sync, got %v", *def2.ScheduleCron)
	}
	if def2.ToolScope != "read_write" {
		t.Errorf("tool_scope = %q, want read_write after update", def2.ToolScope)
	}
}

// TestUniqueCustomWorkflowSlug confirms a duplicate name yields a de-collided
// slug rather than a create failure.
func TestUniqueCustomWorkflowSlug(t *testing.T) {
	svc := newTestSvc(t)
	r := customWorkflowRouter(svc)

	mk := func() *httptest.ResponseRecorder {
		return postForm(t, r, "/-/custom-workflows", url.Values{
			"name":            {"Duplicate Name"},
			"prompt":          {"do something"},
			"trigger_on_sync": {"true"},
		})
	}
	first := mk()
	if first.Code != http.StatusOK {
		t.Fatalf("first create: %d %s", first.Code, first.Body.String())
	}
	second := mk()
	if second.Code != http.StatusOK {
		t.Fatalf("second create: %d %s", second.Code, second.Body.String())
	}
	var a, b struct {
		Slug string `json:"slug"`
	}
	_ = json.Unmarshal(first.Body.Bytes(), &a)
	_ = json.Unmarshal(second.Body.Bytes(), &b)
	if a.Slug == b.Slug {
		t.Fatalf("expected distinct slugs, both = %q", a.Slug)
	}
	if a.Slug != "duplicate-name" || b.Slug != "duplicate-name-2" {
		t.Fatalf("slugs = %q, %q; want duplicate-name, duplicate-name-2", a.Slug, b.Slug)
	}
}
