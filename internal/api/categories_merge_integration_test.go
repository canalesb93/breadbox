//go:build integration

// Integration tests for POST /api/v1/categories/{id}/merge.
//
// The handler currently returns 204 No Content on success — there is no
// JSON body. The matching docs/api-reference.md entry doesn't claim a
// response shape, so 204 is the canonical contract; these tests pin it down.
//
// Run with: DATABASE_URL=... go test -tags integration -count=1 -p 1 -v -run MergeCategories ./internal/api/...

package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/testutil"
)

// --- helpers ---------------------------------------------------------------

// createCategoryViaAPI POSTs and returns the new category's UUID id.
func createCategoryViaAPI(t *testing.T, env *testEnv, slug, displayName string) string {
	t.Helper()
	resp := env.doPost(t, "/api/v1/categories", map[string]any{
		"slug":         slug,
		"display_name": displayName,
	})
	assertStatus(t, resp, http.StatusCreated)
	var cat map[string]any
	parseJSON(t, resp, &cat)
	id, _ := cat["id"].(string)
	if id == "" {
		t.Fatalf("created category has no id: %v", cat)
	}
	return id
}

// --- tests -----------------------------------------------------------------

func TestMergeCategories_ResponseShape(t *testing.T) {
	env := setupTestEnv(t)
	seedUncategorized(t, env.Queries)

	srcID := createCategoryViaAPI(t, env, "src_shape", "Source Shape")
	tgtID := createCategoryViaAPI(t, env, "tgt_shape", "Target Shape")

	resp := env.doPost(t, "/api/v1/categories/"+srcID+"/merge", map[string]string{
		"target_id": tgtID,
	})

	// The handler returns 204 No Content. Body must be empty — no JSON shape.
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("want 204 No Content, got %d, body: %s", resp.StatusCode, string(body))
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(body) != 0 {
		t.Errorf("204 response body should be empty, got %d bytes: %q", len(body), string(body))
	}
}

func TestMergeCategories_TargetGetsTransactions(t *testing.T) {
	env := setupTestEnv(t)
	seedUncategorized(t, env.Queries)

	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, env.Queries, conn.ID, "ext_acct_1", "Checking")

	src := testutil.MustCreateCategory(t, env.Queries, "src_xfer", "Source Xfer")
	tgt := testutil.MustCreateCategory(t, env.Queries, "tgt_xfer", "Target Xfer")

	// 2 transactions on source, 1 on target.
	srcTxns := []db.Transaction{
		testutil.MustCreateTransaction(t, env.Queries, acct.ID, "ext_s1", "S1", 100, "2025-03-01"),
		testutil.MustCreateTransaction(t, env.Queries, acct.ID, "ext_s2", "S2", 200, "2025-03-02"),
	}
	tgtTxn := testutil.MustCreateTransaction(t, env.Queries, acct.ID, "ext_t1", "T1", 300, "2025-03-03")

	for _, txn := range srcTxns {
		if _, err := env.Queries.SetTransactionCategoryOverride(context.Background(), db.SetTransactionCategoryOverrideParams{
			ID:         txn.ID,
			CategoryID: src.ID,
		}); err != nil {
			t.Fatalf("attach src txn: %v", err)
		}
	}
	if _, err := env.Queries.SetTransactionCategoryOverride(context.Background(), db.SetTransactionCategoryOverrideParams{
		ID:         tgtTxn.ID,
		CategoryID: tgt.ID,
	}); err != nil {
		t.Fatalf("attach tgt txn: %v", err)
	}

	resp := env.doPost(t, "/api/v1/categories/"+pgconv.FormatUUID(src.ID)+"/merge", map[string]string{
		"target_id": pgconv.FormatUUID(tgt.ID),
	})
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// All 3 transactions should now point at target.
	tgtIDStr := pgconv.FormatUUID(tgt.ID)
	for _, txn := range append(srcTxns, tgtTxn) {
		got, err := env.Queries.GetTransaction(context.Background(), txn.ID)
		if err != nil {
			t.Fatalf("get txn %s: %v", pgconv.FormatUUID(txn.ID), err)
		}
		if pgconv.FormatUUID(got.CategoryID) != tgtIDStr {
			t.Errorf("txn %s: want category %s, got %s",
				pgconv.FormatUUID(txn.ID), tgtIDStr, pgconv.FormatUUID(got.CategoryID))
		}
	}
}

func TestMergeCategories_RemovesSource(t *testing.T) {
	env := setupTestEnv(t)
	seedUncategorized(t, env.Queries)

	srcID := createCategoryViaAPI(t, env, "src_gone", "Source Gone")
	tgtID := createCategoryViaAPI(t, env, "tgt_gone", "Target Gone")

	resp := env.doPost(t, "/api/v1/categories/"+srcID+"/merge", map[string]string{
		"target_id": tgtID,
	})
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// Source must be gone — GET returns 404.
	resp = env.doGet(t, "/api/v1/categories/"+srcID)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")

	// Target must still exist.
	resp = env.doGet(t, "/api/v1/categories/"+tgtID)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestMergeCategories_NotFound_Source(t *testing.T) {
	env := setupTestEnv(t)
	seedUncategorized(t, env.Queries)

	tgtID := createCategoryViaAPI(t, env, "tgt_only", "Target Only")

	resp := env.doPost(t, "/api/v1/categories/00000000-0000-0000-0000-000000000000/merge", map[string]string{
		"target_id": tgtID,
	})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestMergeCategories_NotFound_Target(t *testing.T) {
	env := setupTestEnv(t)
	seedUncategorized(t, env.Queries)

	srcID := createCategoryViaAPI(t, env, "src_only", "Source Only")

	resp := env.doPost(t, "/api/v1/categories/"+srcID+"/merge", map[string]string{
		"target_id": "00000000-0000-0000-0000-000000000000",
	})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestMergeCategories_RejectsSelfMerge(t *testing.T) {
	env := setupTestEnv(t)
	seedUncategorized(t, env.Queries)

	srcID := createCategoryViaAPI(t, env, "src_self", "Source Self")

	resp := env.doPost(t, "/api/v1/categories/"+srcID+"/merge", map[string]string{
		"target_id": srcID,
	})
	// Handler maps ErrInvalidParameter → 400 VALIDATION_ERROR (see categories.go).
	readErrorCode(t, resp, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestMergeCategories_RequiresWriteScope(t *testing.T) {
	// Seed data + categories with the default write-scope env, then mint a
	// read-only key against the same service to issue the merge call. Avoids
	// the cross-env DB-truncate trap (each setup* call truncates).
	env := setupTestEnv(t)
	seedUncategorized(t, env.Queries)
	srcID := createCategoryViaAPI(t, env, "src_scope", "Source Scope")
	tgtID := createCategoryViaAPI(t, env, "tgt_scope", "Target Scope")

	readKey, err := env.Service.CreateAPIKeyLegacy(t.Context(), "readonly", "read_only")
	if err != nil {
		t.Fatalf("create read-only key: %v", err)
	}

	body := []byte(`{"target_id":"` + tgtID + `"}`)
	req, err := http.NewRequest("POST", env.Server.URL+"/api/v1/categories/"+srcID+"/merge", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("X-API-Key", readKey.PlaintextKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("want 403, got %d, body: %s", resp.StatusCode, string(respBody))
	}
	resp.Body.Close()
}
