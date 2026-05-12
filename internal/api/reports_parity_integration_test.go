//go:build integration && !lite

package api

import (
	"net/http"
	"testing"

	"breadbox/internal/service"
)

// ============================================================
// Reports parity — REST endpoints mirror admin /-/reports/*
// ============================================================

func createReportForTest(t *testing.T, env *testEnv, title string) string {
	t.Helper()
	resp := env.doPost(t, "/api/v1/reports", map[string]any{
		"title": title,
		"body":  "body for " + title,
	})
	assertStatus(t, resp, http.StatusCreated)
	var report map[string]any
	parseJSON(t, resp, &report)
	id, _ := report["id"].(string)
	if id == "" {
		t.Fatalf("created report missing id: %#v", report)
	}
	return id
}

func TestGetReport_Success(t *testing.T) {
	env := setupTestEnv(t)
	id := createReportForTest(t, env, "Inspect Me")

	resp := env.doGet(t, "/api/v1/reports/"+id)
	assertStatus(t, resp, http.StatusOK)

	var got map[string]any
	parseJSON(t, resp, &got)
	if got["id"] != id {
		t.Errorf("id mismatch: got %v, want %v", got["id"], id)
	}
	if got["title"] != "Inspect Me" {
		t.Errorf("title mismatch: got %v", got["title"])
	}
}

func TestGetReport_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doGet(t, "/api/v1/reports/00000000-0000-0000-0000-000000000001")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestGetReport_AllowsReadScope(t *testing.T) {
	// Build a read-only env first (this truncates), then seed a report
	// straight through the service layer using the same shared pool. Using
	// setupTestEnv after setupReadOnlyEnv would trip a second truncate and
	// wipe the seeded data.
	env := setupReadOnlyEnv(t)
	created, err := env.Service.CreateAgentReport(t.Context(), "Visible to read-only", "body",
		service.Actor{Type: "agent", ID: "test-agent", Name: "Test"}, "info", nil, "", "")
	if err != nil {
		t.Fatalf("CreateAgentReport: %v", err)
	}

	resp := env.doGet(t, "/api/v1/reports/"+created.ID)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestMarkUnread_Success(t *testing.T) {
	env := setupTestEnv(t)
	id := createReportForTest(t, env, "Read then unread")

	// Mark as read first
	resp := env.doPatch(t, "/api/v1/reports/"+id+"/read", nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Mark unread
	resp = env.doPatch(t, "/api/v1/reports/"+id+"/unread", nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// GET should show read_at == nil
	resp = env.doGet(t, "/api/v1/reports/"+id)
	assertStatus(t, resp, http.StatusOK)
	var got map[string]any
	parseJSON(t, resp, &got)
	if got["read_at"] != nil {
		t.Errorf("expected read_at to be nil after unread, got %v", got["read_at"])
	}
}

func TestMarkUnread_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doPatch(t, "/api/v1/reports/00000000-0000-0000-0000-000000000001/unread", nil)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestMarkAllRead_FlipsAll(t *testing.T) {
	env := setupTestEnv(t)
	for i := 0; i < 3; i++ {
		createReportForTest(t, env, "Bulk read")
	}

	// Confirm baseline: 3 unread
	resp := env.doGet(t, "/api/v1/reports/unread-count")
	assertStatus(t, resp, http.StatusOK)
	var counts map[string]any
	parseJSON(t, resp, &counts)
	if counts["unread_count"].(float64) < 3 {
		t.Fatalf("expected at least 3 unread reports, got %v", counts["unread_count"])
	}

	// Mark all read
	resp = env.doPost(t, "/api/v1/reports/read-all", nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// All read now
	resp = env.doGet(t, "/api/v1/reports/unread-count")
	assertStatus(t, resp, http.StatusOK)
	parseJSON(t, resp, &counts)
	if counts["unread_count"].(float64) != 0 {
		t.Errorf("expected 0 unread after read-all, got %v", counts["unread_count"])
	}
}

func TestDeleteReport_Success(t *testing.T) {
	env := setupTestEnv(t)
	id := createReportForTest(t, env, "Delete me")

	resp := env.doDelete(t, "/api/v1/reports/"+id)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// GET should now return 404
	resp = env.doGet(t, "/api/v1/reports/"+id)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestDeleteReport_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doDelete(t, "/api/v1/reports/00000000-0000-0000-0000-000000000001")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestReportsParity_RequireWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)

	// PATCH unread
	resp := env.doPatch(t, "/api/v1/reports/00000000-0000-0000-0000-000000000001/unread", nil)
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")

	// POST read-all
	resp = env.doPost(t, "/api/v1/reports/read-all", nil)
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")

	// DELETE
	resp = env.doDelete(t, "/api/v1/reports/00000000-0000-0000-0000-000000000001")
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}
