//go:build integration

// Integration tests for POST /api/v1/rules/batch — REST sibling of the MCP
// `batch_create_rules` tool. Run with:
//
//	DATABASE_URL="postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable" \
//	  go test -tags integration -count=1 -p 1 -v ./internal/api/... -run TestBatchCreateRules_
package api

import (
	"net/http"
	"testing"

	"breadbox/internal/testutil"
)

// TestBatchCreateRules_Success creates three valid rules in one call and
// asserts they all land in the DB and the per-op envelope reports
// succeeded=3, failed=0.
func TestBatchCreateRules_Success(t *testing.T) {
	env := setupTestEnv(t)
	_ = testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")
	_ = testutil.MustCreateCategory(t, env.Queries, "transport", "Transport")
	_ = testutil.MustCreateCategory(t, env.Queries, "dining", "Dining")

	body := map[string]any{
		"rules": []map[string]any{
			{
				"name":          "Whole Foods → Groceries",
				"category_slug": "groceries",
				"conditions":    map[string]any{"field": "provider_name", "op": "contains", "value": "whole foods"},
			},
			{
				"name":          "Uber → Transport",
				"category_slug": "transport",
				"conditions":    map[string]any{"field": "provider_name", "op": "contains", "value": "uber"},
			},
			{
				"name":          "Restaurants → Dining",
				"category_slug": "dining",
				"conditions":    map[string]any{"field": "provider_category_primary", "op": "eq", "value": "FOOD_AND_DRINK"},
			},
		},
	}

	resp := env.doPost(t, "/api/v1/rules/batch", body)
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		Results []struct {
			Index  int    `json:"index"`
			Status string `json:"status"`
			RuleID string `json:"rule_id"`
		} `json:"results"`
		Succeeded int  `json:"succeeded"`
		Failed    int  `json:"failed"`
		Aborted   bool `json:"aborted"`
	}
	parseJSON(t, resp, &out)

	if out.Succeeded != 3 || out.Failed != 0 {
		t.Fatalf("want succeeded=3 failed=0, got succeeded=%d failed=%d (results=%+v)", out.Succeeded, out.Failed, out.Results)
	}
	if out.Aborted {
		t.Fatalf("want aborted=false, got true")
	}
	if len(out.Results) != 3 {
		t.Fatalf("want 3 results, got %d", len(out.Results))
	}
	for i, r := range out.Results {
		if r.Status != "ok" || r.RuleID == "" {
			t.Fatalf("result[%d]: want status=ok with rule_id, got %+v", i, r)
		}
	}

	// Verify all rules are in the registry.
	listResp := env.doGet(t, "/api/v1/rules?limit=100")
	assertStatus(t, listResp, http.StatusOK)
	var listOut struct {
		Rules []map[string]any `json:"rules"`
	}
	parseJSON(t, listResp, &listOut)
	if len(listOut.Rules) != 3 {
		t.Fatalf("want 3 rules in registry, got %d", len(listOut.Rules))
	}
}

// TestBatchCreateRules_PartialFailure_Continue runs one valid rule and one
// rule with a bad category slug; continue mode commits the first rule and
// reports the second op's per-row error inside results[].
func TestBatchCreateRules_PartialFailure_Continue(t *testing.T) {
	env := setupTestEnv(t)
	_ = testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")

	body := map[string]any{
		"on_error": "continue",
		"rules": []map[string]any{
			{
				"name":          "Valid Grocery Rule",
				"category_slug": "groceries",
				"conditions":    map[string]any{"field": "provider_name", "op": "contains", "value": "trader"},
			},
			{
				"name":          "Bad Category Rule",
				"category_slug": "nonexistent_category_slug",
				"conditions":    map[string]any{"field": "provider_name", "op": "eq", "value": "x"},
			},
		},
	}

	resp := env.doPost(t, "/api/v1/rules/batch", body)
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		Results []struct {
			Index  int    `json:"index"`
			Status string `json:"status"`
			RuleID string `json:"rule_id"`
			Error  *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error,omitempty"`
		} `json:"results"`
		Succeeded int  `json:"succeeded"`
		Failed    int  `json:"failed"`
		Aborted   bool `json:"aborted"`
	}
	parseJSON(t, resp, &out)

	if out.Succeeded != 1 || out.Failed != 1 {
		t.Fatalf("want succeeded=1 failed=1, got %+v", out)
	}
	if out.Aborted {
		t.Fatalf("continue mode must not set aborted")
	}
	if len(out.Results) != 2 {
		t.Fatalf("want 2 results, got %d", len(out.Results))
	}
	if out.Results[0].Status != "ok" {
		t.Fatalf("want first op ok, got %+v", out.Results[0])
	}
	if out.Results[1].Status != "error" || out.Results[1].Error == nil || out.Results[1].Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("want second op VALIDATION_ERROR, got %+v", out.Results[1])
	}

	// Verify the first rule landed.
	listResp := env.doGet(t, "/api/v1/rules?limit=10")
	var listOut struct {
		Rules []map[string]any `json:"rules"`
	}
	parseJSON(t, listResp, &listOut)
	if len(listOut.Rules) != 1 {
		t.Fatalf("continue mode: want 1 rule in registry, got %d", len(listOut.Rules))
	}
}

// TestBatchCreateRules_Abort same shape as continue mode but with abort.
// The whole batch rolls back: the first (valid) rule must NOT persist.
func TestBatchCreateRules_Abort(t *testing.T) {
	env := setupTestEnv(t)
	_ = testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")

	body := map[string]any{
		"on_error": "abort",
		"rules": []map[string]any{
			{
				"name":          "Valid Rule",
				"category_slug": "groceries",
				"conditions":    map[string]any{"field": "provider_name", "op": "contains", "value": "trader"},
			},
			{
				"name":          "Bad Category Rule",
				"category_slug": "nonexistent_category_slug",
				"conditions":    map[string]any{"field": "provider_name", "op": "eq", "value": "x"},
			},
		},
	}

	resp := env.doPost(t, "/api/v1/rules/batch", body)
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		Results []struct {
			Status string `json:"status"`
			Error  *struct {
				Code string `json:"code"`
			} `json:"error,omitempty"`
		} `json:"results"`
		Succeeded int  `json:"succeeded"`
		Failed    int  `json:"failed"`
		Aborted   bool `json:"aborted"`
	}
	parseJSON(t, resp, &out)

	if !out.Aborted {
		t.Fatalf("want aborted=true on abort failure, got %+v", out)
	}
	// Abort rolls back the previously-created rule, so succeeded==0.
	if out.Succeeded != 0 || out.Failed != 1 {
		t.Fatalf("want succeeded=0 failed=1 after rollback, got %+v", out)
	}
	if len(out.Results) != 2 {
		t.Fatalf("want 2 results (both before-failure ops), got %d", len(out.Results))
	}
	if out.Results[1].Status != "error" || out.Results[1].Error == nil || out.Results[1].Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("want second op VALIDATION_ERROR, got %+v", out.Results[1])
	}

	// Verify nothing landed.
	listResp := env.doGet(t, "/api/v1/rules?limit=10")
	var listOut struct {
		Rules []map[string]any `json:"rules"`
	}
	parseJSON(t, listResp, &listOut)
	if len(listOut.Rules) != 0 {
		t.Fatalf("abort mode should have rolled back; want 0 rules, got %d", len(listOut.Rules))
	}
}

// TestBatchCreateRules_RejectsEmpty — empty rules array is a top-level
// 400 INVALID_PARAMETER.
func TestBatchCreateRules_RejectsEmpty(t *testing.T) {
	env := setupTestEnv(t)

	body := map[string]any{"rules": []map[string]any{}}
	resp := env.doPost(t, "/api/v1/rules/batch", body)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

// TestBatchCreateRules_RejectsTooMany — > 50 rules returns 400.
func TestBatchCreateRules_RejectsTooMany(t *testing.T) {
	env := setupTestEnv(t)

	rules := make([]map[string]any, 51)
	for i := range rules {
		rules[i] = map[string]any{
			"name":          "Rule " + string(rune('a'+(i%26))),
			"category_slug": "groceries",
		}
	}
	body := map[string]any{"rules": rules}
	resp := env.doPost(t, "/api/v1/rules/batch", body)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

// TestBatchCreateRules_RequiresWriteScope — read-only key is blocked.
func TestBatchCreateRules_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)

	body := map[string]any{
		"rules": []map[string]any{
			{"name": "x", "category_slug": "groceries"},
		},
	}
	resp := env.doPost(t, "/api/v1/rules/batch", body)
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}
