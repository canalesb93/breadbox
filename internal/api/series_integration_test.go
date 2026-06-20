//go:build integration && !lite

// Integration tests for the REST recurring-series (subscriptions) endpoints.
//
//	DATABASE_URL="postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable" \
//	go test -tags integration -count=1 -p 1 -v ./internal/api/... -run TestAPI_Series
package api

import (
	"net/http"
	"testing"

	"breadbox/internal/service"
)

func TestAPI_Series_ListGetAssignPatch(t *testing.T) {
	env := setupTestEnv(t)

	// Assign (mint by name) via POST /api/v1/series.
	resp := env.doPost(t, "/api/v1/series", map[string]any{
		"series_name":       "Netflix",
		"create_if_missing": true,
		"type":              "subscription",
	})
	assertStatus(t, resp, http.StatusOK)
	var created struct {
		ShortID string `json:"short_id"`
		Name    string `json:"name"`
		Type    string `json:"type"`
	}
	parseJSON(t, resp, &created)
	if created.Name != "Netflix" || created.Type != "subscription" {
		t.Fatalf("assign result = %+v, want Netflix/subscription", created)
	}

	// List.
	resp = env.doGet(t, "/api/v1/series")
	assertStatus(t, resp, http.StatusOK)
	var list struct {
		Series []struct {
			ShortID string `json:"short_id"`
			Name    string `json:"name"`
			Type    string `json:"type"`
		} `json:"series"`
	}
	parseJSON(t, resp, &list)
	if len(list.Series) != 1 || list.Series[0].Name != "Netflix" {
		t.Fatalf("list = %+v, want one Netflix series", list.Series)
	}

	// Get by short_id.
	resp = env.doGet(t, "/api/v1/series/"+created.ShortID)
	assertStatus(t, resp, http.StatusOK)

	// Patch: rename + retype.
	resp = env.doPatch(t, "/api/v1/series/"+created.ShortID, map[string]any{
		"name": "Netflix Premium",
		"type": "bill",
	})
	assertStatus(t, resp, http.StatusOK)
	var patched struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	parseJSON(t, resp, &patched)
	if patched.Name != "Netflix Premium" || patched.Type != "bill" {
		t.Fatalf("patch result = %+v, want Netflix Premium/bill", patched)
	}

	// Empty patch → 400.
	resp = env.doPatch(t, "/api/v1/series/"+created.ShortID, map[string]any{})
	assertStatus(t, resp, http.StatusBadRequest)

	// Unknown (well-formed) short_id → 404.
	resp = env.doGet(t, "/api/v1/series/zzzzzzzz")
	assertStatus(t, resp, http.StatusNotFound)
}

// Ensure the service constant stays referenced so an accidental rename surfaces
// in this package's compile.
var _ = service.SeriesTypeSubscription
