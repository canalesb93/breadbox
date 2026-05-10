//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/testutil"
)

// sampleCSV is a small, deterministic CSV used across all the CSV import
// integration tests. Two distinct rows; trivially detectable column
// mapping (date / amount / description).
const sampleCSV = "date,amount,description\n" +
	"2025-01-15,12.50,Coffee Shop\n" +
	"2025-01-16,42.00,Grocery Store\n"

// sampleCSVColumnMapping is the column mapping that matches sampleCSV.
func sampleCSVColumnMapping() map[string]int {
	return map[string]int{"date": 0, "amount": 1, "description": 2}
}

// sampleCSVMultipart builds a multipart/form-data body for sampleCSV plus
// the supplied form fields. Returns the body, content-type header value,
// and any error.
func sampleCSVMultipart(t *testing.T, body string, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	part, err := mw.CreateFormFile("file", "sample.csv")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(part, body); err != nil {
		t.Fatalf("write form file: %v", err)
	}

	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("write field %q: %v", k, err)
		}
	}

	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return &buf, mw.FormDataContentType()
}

// doRaw sends an arbitrary request body with an explicit Content-Type. Use
// this for multipart and oversized payload tests.
func (e *testEnv) doRaw(t *testing.T, method, path, contentType string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, e.Server.URL+path, body)
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

// ------------------------------------------------------------
// Preview
// ------------------------------------------------------------

func TestCSVPreview_JSON_Success(t *testing.T) {
	env := setupTestEnv(t)

	body := map[string]any{
		"csv_base64": base64.StdEncoding.EncodeToString([]byte(sampleCSV)),
	}
	resp := env.doPost(t, "/api/v1/connections/csv/preview", body)
	assertStatus(t, resp, http.StatusOK)

	var out map[string]any
	parseJSON(t, resp, &out)

	headers, _ := out["headers"].([]any)
	if len(headers) != 3 {
		t.Fatalf("want 3 headers, got %d (%v)", len(headers), headers)
	}
	if headers[0] != "date" {
		t.Fatalf("want first header 'date', got %v", headers[0])
	}
	preview, _ := out["preview_rows"].([]any)
	if len(preview) != 2 {
		t.Fatalf("want 2 preview rows, got %d", len(preview))
	}
	mapping, _ := out["inferred_mapping"].(map[string]any)
	if mapping == nil {
		t.Fatalf("want inferred_mapping, got nil")
	}
	// Generic header detection should at least find date + description.
	if _, ok := mapping["date"]; !ok {
		t.Fatalf("want inferred_mapping.date, got %v", mapping)
	}
}

func TestCSVPreview_Multipart_Success(t *testing.T) {
	env := setupTestEnv(t)

	body, ct := sampleCSVMultipart(t, sampleCSV, nil)
	resp := env.doRaw(t, "POST", "/api/v1/connections/csv/preview", ct, body)
	assertStatus(t, resp, http.StatusOK)

	var out map[string]any
	parseJSON(t, resp, &out)

	headers, _ := out["headers"].([]any)
	if len(headers) != 3 {
		t.Fatalf("want 3 headers, got %d", len(headers))
	}
}

func TestCSVPreview_BadCSV(t *testing.T) {
	env := setupTestEnv(t)

	// Single row — fails the parser's "at least 2 rows" check.
	body := map[string]any{
		"csv_base64": base64.StdEncoding.EncodeToString([]byte("date\n")),
	}
	resp := env.doPost(t, "/api/v1/connections/csv/preview", body)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

// ------------------------------------------------------------
// Import
// ------------------------------------------------------------

func TestCSVImport_JSON_NewConnection_Success(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	seedUncategorized(t, env.Queries)

	body := map[string]any{
		"csv_base64":     base64.StdEncoding.EncodeToString([]byte(sampleCSV)),
		"user_id":        pgconv.FormatUUID(user.ID),
		"account_name":   "Test Bank",
		"column_mapping": sampleCSVColumnMapping(),
		"date_format":    "2006-01-02",
	}
	resp := env.doPost(t, "/api/v1/connections/csv/import", body)
	assertStatus(t, resp, http.StatusCreated)

	var out csvImportResponse
	parseJSON(t, resp, &out)

	if out.ConnectionID == "" {
		t.Fatalf("want connection_id, got empty")
	}
	if out.AccountID == "" {
		t.Fatalf("want account_id, got empty")
	}
	if out.ImportedTransactions != 2 {
		t.Fatalf("want 2 imported, got %d (%+v)", out.ImportedTransactions, out)
	}

	// Confirm the connection exists and is a CSV connection.
	var provider string
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT provider::text FROM bank_connections WHERE user_id = $1`, user.ID).Scan(&provider); err != nil {
		t.Fatalf("read connection: %v", err)
	}
	if provider != "csv" {
		t.Fatalf("want csv provider, got %s", provider)
	}
}

func TestCSVImport_JSON_ExistingConnection(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Bob")
	seedUncategorized(t, env.Queries)

	// First import creates the connection.
	first := map[string]any{
		"csv_base64":     base64.StdEncoding.EncodeToString([]byte(sampleCSV)),
		"user_id":        pgconv.FormatUUID(user.ID),
		"account_name":   "Existing Bank",
		"column_mapping": sampleCSVColumnMapping(),
		"date_format":    "2006-01-02",
	}
	resp := env.doPost(t, "/api/v1/connections/csv/import", first)
	assertStatus(t, resp, http.StatusCreated)
	var firstOut csvImportResponse
	parseJSON(t, resp, &firstOut)

	// Second import targets the same connection by short_id and adds a new
	// row. Use a distinct date+description so it doesn't collide on the
	// dedup hash.
	extraCSV := "date,amount,description\n" +
		"2025-02-01,99.00,Brand New Charge\n"
	second := map[string]any{
		"csv_base64":     base64.StdEncoding.EncodeToString([]byte(extraCSV)),
		"connection_id":  firstOut.ConnectionID,
		"column_mapping": sampleCSVColumnMapping(),
		"date_format":    "2006-01-02",
	}
	resp = env.doPost(t, "/api/v1/connections/csv/import", second)
	assertStatus(t, resp, http.StatusCreated)

	var secondOut csvImportResponse
	parseJSON(t, resp, &secondOut)

	if secondOut.ConnectionID != firstOut.ConnectionID {
		t.Fatalf("want same connection_id, got first=%s second=%s", firstOut.ConnectionID, secondOut.ConnectionID)
	}
	if secondOut.ImportedTransactions != 1 {
		t.Fatalf("want 1 imported, got %d", secondOut.ImportedTransactions)
	}
}

func TestCSVImport_DeduplicatesProviderTransactionID(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Carol")
	seedUncategorized(t, env.Queries)

	body := map[string]any{
		"csv_base64":     base64.StdEncoding.EncodeToString([]byte(sampleCSV)),
		"user_id":        pgconv.FormatUUID(user.ID),
		"account_name":   "Dedup Bank",
		"column_mapping": sampleCSVColumnMapping(),
		"date_format":    "2006-01-02",
	}
	resp := env.doPost(t, "/api/v1/connections/csv/import", body)
	assertStatus(t, resp, http.StatusCreated)
	var first csvImportResponse
	parseJSON(t, resp, &first)
	if first.ImportedTransactions != 2 {
		t.Fatalf("first import: want 2 new, got %d", first.ImportedTransactions)
	}

	// Re-import the same CSV against the same connection — every row
	// generates the same dedup hash, so the upsert path runs as a no-op.
	// The strong invariant is that the *physical row count* doesn't grow,
	// regardless of how the service classifies the row internally.
	body["connection_id"] = first.ConnectionID
	delete(body, "user_id")
	resp = env.doPost(t, "/api/v1/connections/csv/import", body)
	assertStatus(t, resp, http.StatusCreated)
	var second csvImportResponse
	parseJSON(t, resp, &second)

	// The transactions table should still have exactly 2 rows for this
	// account. This is the real dedup contract — counts in the response
	// payload are advisory.
	var txnCount int
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM transactions t
         JOIN accounts a ON a.id = t.account_id
         JOIN bank_connections bc ON bc.id = a.connection_id
         WHERE bc.user_id = $1`, user.ID).Scan(&txnCount); err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if txnCount != 2 {
		t.Fatalf("dedup failed: want 2 transactions in DB, got %d (response: %+v)", txnCount, second)
	}
}

func TestCSVImport_Multipart_Success(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Dave")
	seedUncategorized(t, env.Queries)

	mappingJSON, _ := json.Marshal(sampleCSVColumnMapping())
	body, ct := sampleCSVMultipart(t, sampleCSV, map[string]string{
		"user_id":        pgconv.FormatUUID(user.ID),
		"account_name":   "Multipart Bank",
		"column_mapping": string(mappingJSON),
		"date_format":    "2006-01-02",
	})
	resp := env.doRaw(t, "POST", "/api/v1/connections/csv/import", ct, body)
	assertStatus(t, resp, http.StatusCreated)

	var out csvImportResponse
	parseJSON(t, resp, &out)
	if out.ImportedTransactions != 2 {
		t.Fatalf("want 2 imported, got %d", out.ImportedTransactions)
	}
}

func TestCSVImport_BadConnection_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	seedUncategorized(t, env.Queries)

	body := map[string]any{
		"csv_base64":     base64.StdEncoding.EncodeToString([]byte(sampleCSV)),
		"connection_id":  "00000000-0000-0000-0000-000000000000",
		"column_mapping": sampleCSVColumnMapping(),
		"date_format":    "2006-01-02",
	}
	resp := env.doPost(t, "/api/v1/connections/csv/import", body)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestCSVImport_PayloadTooLarge(t *testing.T) {
	env := setupTestEnv(t)

	// Use a small, valid-looking JSON body whose base64 payload decodes
	// to something larger than maxCSVRESTUploadSize. The base64 form
	// stays well below MaxBytesReader's 50MB cap (so the request
	// reaches the handler), but the post-decode length check trips and
	// returns 413. This keeps the test fast and avoids HTTP-level
	// flakiness around streaming a huge body.
	//
	// We trigger the check by lying about the encoded length: send a
	// tiny payload but tell the handler it represents something
	// enormous via a custom test handler. Simpler: just exceed the
	// MaxBytesReader cap directly, but with a body that's only a few KB
	// over the limit so the test runs quickly.
	overflow := make([]byte, maxCSVRESTUploadSize+128)
	for i := range overflow {
		overflow[i] = 'A'
	}
	resp := env.doRaw(t, "POST", "/api/v1/connections/csv/preview",
		"application/json", bytes.NewReader(overflow))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 413, got %d, body: %s", resp.StatusCode, bodyBytes)
	}
}

func TestCSVImport_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)

	body := map[string]any{
		"csv_base64": base64.StdEncoding.EncodeToString([]byte(sampleCSV)),
	}

	resp := env.doPost(t, "/api/v1/connections/csv/preview", body)
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")

	resp = env.doPost(t, "/api/v1/connections/csv/import", body)
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

func TestCSVImport_PreservesCategoryOverride(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Eve")
	uncat := seedUncategorized(t, env.Queries)
	groceries := testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")

	// Initial import.
	body := map[string]any{
		"csv_base64":     base64.StdEncoding.EncodeToString([]byte(sampleCSV)),
		"user_id":        pgconv.FormatUUID(user.ID),
		"account_name":   "Override Bank",
		"column_mapping": sampleCSVColumnMapping(),
		"date_format":    "2006-01-02",
	}
	resp := env.doPost(t, "/api/v1/connections/csv/import", body)
	assertStatus(t, resp, http.StatusCreated)
	var first csvImportResponse
	parseJSON(t, resp, &first)
	if first.ImportedTransactions != 2 {
		t.Fatalf("first import: want 2 imported, got %+v", first)
	}

	// Pick the "Grocery Store" transaction by description and force a
	// manual category override. The rest of the test asserts that
	// re-importing the same CSV does NOT clobber that override.
	var groceriesTxnID = ""
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT id::text FROM transactions WHERE provider_name = 'Grocery Store'`).Scan(&groceriesTxnID); err != nil {
		t.Fatalf("locate grocery txn: %v", err)
	}

	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE transactions SET category_id = $1, category_override = TRUE WHERE id = $2`,
		groceries.ID, groceriesTxnID); err != nil {
		t.Fatalf("set override: %v", err)
	}

	// Sanity: the row really is overridden.
	var beforeOverride bool
	var beforeCategory string
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT category_override, category_id::text FROM transactions WHERE id = $1`,
		groceriesTxnID).Scan(&beforeOverride, &beforeCategory); err != nil {
		t.Fatalf("read pre-state: %v", err)
	}
	if !beforeOverride {
		t.Fatalf("override flag did not stick")
	}

	// Re-import the same CSV.
	body["connection_id"] = first.ConnectionID
	delete(body, "user_id")
	resp = env.doPost(t, "/api/v1/connections/csv/import", body)
	assertStatus(t, resp, http.StatusCreated)

	// Override must still be true and category must still point at
	// groceries — not be reset to uncategorized.
	var afterOverride bool
	var afterCategory string
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT category_override, category_id::text FROM transactions WHERE id = $1`,
		groceriesTxnID).Scan(&afterOverride, &afterCategory); err != nil {
		t.Fatalf("read post-state: %v", err)
	}
	if !afterOverride {
		t.Fatalf("re-import cleared category_override flag")
	}
	if afterCategory != beforeCategory {
		t.Fatalf("re-import changed category_id (was %s, now %s); should have respected override", beforeCategory, afterCategory)
	}
	if afterCategory == pgconv.FormatUUID(uncat.ID) {
		t.Fatalf("re-import reset to uncategorized; override was sacred")
	}
}


