//go:build integration

// Integration tests for the TSV import/export endpoints:
//   - GET  /api/v1/categories/export
//   - POST /api/v1/categories/import
//
// Run with: DATABASE_URL=... go test -tags integration -count=1 -p 1 -v -run Categor ./internal/api/...

package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// --- helpers ---------------------------------------------------------------

// doRawBody POSTs a raw byte body with no Content-Type assumption — used for
// TSV requests where doJSON's marshalling and content-type defaulting would
// get in the way.
func (e *testEnv) doRawBody(t *testing.T, method, path, contentType string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, e.Server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("X-API-Key", e.APIKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// readBody reads the full response body and closes it.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}

// parseTSVBody splits a TSV body into a header row and data rows (each as
// tab-separated columns). Trailing blank lines are dropped.
func parseTSVBody(t *testing.T, body string) (header []string, rows [][]string) {
	t.Helper()
	lines := strings.Split(body, "\n")
	// Drop trailing empty lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		t.Fatalf("empty TSV body")
	}
	header = strings.Split(lines[0], "\t")
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rows = append(rows, strings.Split(line, "\t"))
	}
	return header, rows
}

// findRow returns the row whose first column equals slug, or nil if absent.
func findRow(rows [][]string, slug string) []string {
	for _, r := range rows {
		if len(r) > 0 && r[0] == slug {
			return r
		}
	}
	return nil
}

// --- export ----------------------------------------------------------------

func TestExportCategories_Empty(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/categories/export")
	assertStatus(t, resp, http.StatusOK)
	body := readBody(t, resp)

	header, rows := parseTSVBody(t, body)
	if len(rows) != 0 {
		t.Errorf("want 0 data rows on empty env, got %d: %v", len(rows), rows)
	}
	wantHeader := []string{"slug", "display_name", "parent_slug", "icon", "color", "sort_order", "hidden", "merge_into"}
	if len(header) != len(wantHeader) {
		t.Fatalf("header length: want %d, got %d (%v)", len(wantHeader), len(header), header)
	}
	for i, col := range wantHeader {
		if header[i] != col {
			t.Errorf("header[%d]: want %q, got %q", i, col, header[i])
		}
	}
}

func TestExportCategories_Multiple(t *testing.T) {
	env := setupTestEnv(t)
	testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")
	testutil.MustCreateCategory(t, env.Queries, "dining", "Dining Out")
	testutil.MustCreateCategory(t, env.Queries, "transport", "Transport")

	resp := env.doGet(t, "/api/v1/categories/export")
	assertStatus(t, resp, http.StatusOK)
	body := readBody(t, resp)

	_, rows := parseTSVBody(t, body)
	if len(rows) != 3 {
		t.Fatalf("want 3 data rows, got %d", len(rows))
	}

	for _, slug := range []string{"groceries", "dining", "transport"} {
		row := findRow(rows, slug)
		if row == nil {
			t.Errorf("row for slug %q missing", slug)
			continue
		}
		if row[1] == "" {
			t.Errorf("row %q: display_name empty", slug)
		}
	}

	// Confirm display_name lands in column 2 ("Dining Out", not "dining").
	dining := findRow(rows, "dining")
	if dining == nil || dining[1] != "Dining Out" {
		t.Errorf("dining row display_name: want 'Dining Out', got %v", dining)
	}
}

func TestExportCategories_TabSeparated(t *testing.T) {
	env := setupTestEnv(t)
	testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")

	resp := env.doGet(t, "/api/v1/categories/export")
	assertStatus(t, resp, http.StatusOK)

	ct := resp.Header.Get("Content-Type")
	if ct != "text/tab-separated-values" {
		t.Errorf("Content-Type: want text/tab-separated-values, got %q", ct)
	}

	body := readBody(t, resp)
	if !strings.Contains(body, "\t") {
		t.Errorf("body should contain tabs, got: %q", body)
	}
	// Header should not be comma-separated.
	firstLine := strings.SplitN(body, "\n", 2)[0]
	if strings.Contains(firstLine, ",") {
		t.Errorf("header line should not contain commas (TSV, not CSV): %q", firstLine)
	}
	tabsInHeader := strings.Count(firstLine, "\t")
	if tabsInHeader < 7 {
		t.Errorf("header should have at least 7 tabs (8 cols), got %d in %q", tabsInHeader, firstLine)
	}
}

func TestExportCategories_HasParentColumn(t *testing.T) {
	env := setupTestEnv(t)
	parent := testutil.MustCreateCategory(t, env.Queries, "food", "Food")
	// Insert child via direct query so we can set ParentID.
	_, err := env.Queries.InsertCategory(context.Background(), db.InsertCategoryParams{
		Slug:        "groceries",
		DisplayName: "Groceries",
		ParentID:    parent.ID,
	})
	if err != nil {
		t.Fatalf("insert child category: %v", err)
	}

	resp := env.doGet(t, "/api/v1/categories/export")
	assertStatus(t, resp, http.StatusOK)
	body := readBody(t, resp)

	header, rows := parseTSVBody(t, body)
	// parent_slug is column index 2.
	parentColIdx := -1
	for i, col := range header {
		if col == "parent_slug" {
			parentColIdx = i
			break
		}
	}
	if parentColIdx < 0 {
		t.Fatalf("parent_slug column missing from header: %v", header)
	}

	child := findRow(rows, "groceries")
	if child == nil {
		t.Fatalf("groceries row missing")
	}
	if child[parentColIdx] != "food" {
		t.Errorf("groceries.parent_slug: want 'food', got %q", child[parentColIdx])
	}

	parentRow := findRow(rows, "food")
	if parentRow == nil {
		t.Fatalf("food row missing")
	}
	if parentRow[parentColIdx] != "" {
		t.Errorf("food.parent_slug: want empty, got %q", parentRow[parentColIdx])
	}

	// Parent must come before child (export ordering invariant).
	parentIdx, childIdx := -1, -1
	for i, r := range rows {
		if r[0] == "food" {
			parentIdx = i
		}
		if r[0] == "groceries" {
			childIdx = i
		}
	}
	if parentIdx >= childIdx {
		t.Errorf("parent should appear before child: parentIdx=%d childIdx=%d", parentIdx, childIdx)
	}
}

func TestExportCategories_AllowsReadScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")

	resp := env.doGet(t, "/api/v1/categories/export")
	assertStatus(t, resp, http.StatusOK)
	body := readBody(t, resp)
	if !strings.Contains(body, "groceries") {
		t.Errorf("export body should include 'groceries'; got: %q", body)
	}
}

// --- import ----------------------------------------------------------------

const importHeader = "slug\tdisplay_name\tparent_slug\ticon\tcolor\tsort_order\thidden\tmerge_into\n"

func TestImportCategories_NewRecords(t *testing.T) {
	env := setupTestEnv(t)
	body := importHeader +
		"groceries\tGroceries\t\t\t\t10\tfalse\t\n" +
		"dining\tDining Out\t\t\t\t20\tfalse\t\n"

	resp := env.doRawBody(t, "POST", "/api/v1/categories/import", "text/tab-separated-values", []byte(body))
	assertStatus(t, resp, http.StatusOK)

	var result service.CategoryImportResult
	parseJSON(t, resp, &result)
	if result.Created != 2 {
		t.Errorf("Created: want 2, got %d (errors: %v)", result.Created, result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	got, err := env.Queries.GetCategoryBySlug(context.Background(), "groceries")
	if err != nil {
		t.Fatalf("groceries not found post-import: %v", err)
	}
	if got.DisplayName != "Groceries" {
		t.Errorf("DisplayName: want 'Groceries', got %q", got.DisplayName)
	}
	if got.SortOrder != 10 {
		t.Errorf("SortOrder: want 10, got %d", got.SortOrder)
	}
}

func TestImportCategories_UpdatesExisting(t *testing.T) {
	env := setupTestEnv(t)
	testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")

	// Re-import with a different display name and sort order.
	body := importHeader + "groceries\tFood & Groceries\t\t\t\t99\ttrue\t\n"
	resp := env.doRawBody(t, "POST", "/api/v1/categories/import", "text/tab-separated-values", []byte(body))
	assertStatus(t, resp, http.StatusOK)

	var result service.CategoryImportResult
	parseJSON(t, resp, &result)
	if result.Updated != 1 {
		t.Errorf("Updated: want 1, got %d (created=%d errors=%v)", result.Updated, result.Created, result.Errors)
	}
	if result.Created != 0 {
		t.Errorf("Created: want 0, got %d", result.Created)
	}

	got, err := env.Queries.GetCategoryBySlug(context.Background(), "groceries")
	if err != nil {
		t.Fatalf("groceries lookup: %v", err)
	}
	if got.DisplayName != "Food & Groceries" {
		t.Errorf("DisplayName: want 'Food & Groceries', got %q", got.DisplayName)
	}
	if got.SortOrder != 99 {
		t.Errorf("SortOrder: want 99, got %d", got.SortOrder)
	}
	if !got.Hidden {
		t.Errorf("Hidden: want true, got false")
	}
}

func TestImportCategories_RejectsBadFormat(t *testing.T) {
	env := setupTestEnv(t)
	// Header has only 3 columns — under the minimum of 7.
	body := "slug\tdisplay_name\tparent_slug\ngroceries\tGroceries\t\n"

	resp := env.doRawBody(t, "POST", "/api/v1/categories/import", "text/tab-separated-values", []byte(body))
	// Service returns IMPORT_ERROR (mapped to 400) on malformed TSV. The doc/code
	// agree that any client-side parse failure is a 400.
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("want 400, got %d, body: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
}

func TestImportCategories_RejectsBadParentSlug(t *testing.T) {
	env := setupTestEnv(t)
	// References a parent_slug that doesn't exist. The handler returns 200 with
	// the error captured in result.Errors (per-row best-effort import).
	body := importHeader + "child\tChild\tnonexistent_parent\t\t\t10\tfalse\t\n"

	resp := env.doRawBody(t, "POST", "/api/v1/categories/import", "text/tab-separated-values", []byte(body))
	assertStatus(t, resp, http.StatusOK)

	var result service.CategoryImportResult
	parseJSON(t, resp, &result)
	if result.Created != 0 {
		t.Errorf("Created: want 0, got %d", result.Created)
	}
	if len(result.Errors) == 0 {
		t.Fatalf("expected per-row error referencing missing parent, got none")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "nonexistent_parent") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("error list should mention 'nonexistent_parent', got: %v", result.Errors)
	}
}

func TestImportCategories_PreservesCategoryOverride(t *testing.T) {
	env := setupTestEnv(t)

	// Seed: connection + account + transaction with a category override.
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, env.Queries, conn.ID, "ext_acct_1", "Checking")
	cat := testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")
	txn := testutil.MustCreateTransaction(t, env.Queries, acct.ID, "ext_txn_1", "Whole Foods", 4500, "2025-03-15")

	// Pin the transaction to groceries with category_override=true.
	if _, err := env.Queries.SetTransactionCategoryOverride(context.Background(), db.SetTransactionCategoryOverrideParams{
		ID:         txn.ID,
		CategoryID: cat.ID,
	}); err != nil {
		t.Fatalf("set override: %v", err)
	}

	// Re-import with a different display name.
	body := importHeader + "groceries\tFood & Groceries\t\t\t\t50\tfalse\t\n"
	resp := env.doRawBody(t, "POST", "/api/v1/categories/import", "text/tab-separated-values", []byte(body))
	assertStatus(t, resp, http.StatusOK)

	var result service.CategoryImportResult
	parseJSON(t, resp, &result)
	if result.Updated != 1 {
		t.Errorf("Updated: want 1, got %d", result.Updated)
	}

	// Verify the transaction still has its override and the same category_id.
	got, err := env.Queries.GetTransaction(context.Background(), txn.ID)
	if err != nil {
		t.Fatalf("get transaction: %v", err)
	}
	if !got.CategoryOverride {
		t.Errorf("category_override: want true, got false (import must not touch transaction overrides)")
	}
	if pgconv.FormatUUID(got.CategoryID) != pgconv.FormatUUID(cat.ID) {
		t.Errorf("category_id changed by import — want %s, got %s", pgconv.FormatUUID(cat.ID), pgconv.FormatUUID(got.CategoryID))
	}
}

func TestImportCategories_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	body := importHeader + "groceries\tGroceries\t\t\t\t10\tfalse\t\n"

	resp := env.doRawBody(t, "POST", "/api/v1/categories/import", "text/tab-separated-values", []byte(body))
	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("want 403, got %d, body: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
}

func TestImportCategories_RoundTrip(t *testing.T) {
	env := setupTestEnv(t)
	parent := testutil.MustCreateCategory(t, env.Queries, "food", "Food")
	if _, err := env.Queries.InsertCategory(context.Background(), db.InsertCategoryParams{
		Slug:        "groceries",
		DisplayName: "Groceries",
		ParentID:    parent.ID,
	}); err != nil {
		t.Fatalf("insert child: %v", err)
	}
	testutil.MustCreateCategory(t, env.Queries, "transport", "Transport")

	// Export.
	resp := env.doGet(t, "/api/v1/categories/export")
	assertStatus(t, resp, http.StatusOK)
	exported := readBody(t, resp)

	// Re-import the exported body — should be a no-op.
	resp = env.doRawBody(t, "POST", "/api/v1/categories/import", "text/tab-separated-values", []byte(exported))
	assertStatus(t, resp, http.StatusOK)

	var result service.CategoryImportResult
	parseJSON(t, resp, &result)
	if result.Created != 0 {
		t.Errorf("round-trip Created: want 0, got %d", result.Created)
	}
	if result.Updated != 0 {
		t.Errorf("round-trip Updated: want 0, got %d (errors=%v)", result.Updated, result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Errorf("round-trip errors: %v", result.Errors)
	}
	if result.Unchanged != 3 {
		t.Errorf("round-trip Unchanged: want 3, got %d", result.Unchanged)
	}
}
