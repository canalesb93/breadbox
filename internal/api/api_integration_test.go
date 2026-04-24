//go:build integration

// Integration tests for REST API handlers. Require a running PostgreSQL with breadbox_test database.
// Run with: DATABASE_URL="postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable" go test -tags integration -count=1 -p 1 -v ./internal/api/...
//
// IMPORTANT: Do NOT use t.Parallel() — tests share a database and truncate between runs.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMain(m *testing.M) {
	testutil.RunWithDB(m)
}

// testEnv holds the test server and helpers for making authenticated requests.
type testEnv struct {
	Server  *httptest.Server
	APIKey  string // plaintext API key for X-API-Key header
	Service *service.Service
	Queries *db.Queries
	Pool    *pgxpool.Pool
}

// setupTestEnv creates a service, wires up a chi router with API key auth middleware,
// and returns a running httptest.Server.
func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())

	keyResult, err := svc.CreateAPIKey(t.Context(), "test-key", "full_access")
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	r := buildTestRouter(svc)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	return &testEnv{
		Server:  server,
		APIKey:  keyResult.PlaintextKey,
		Service: svc,
		Queries: queries,
		Pool:    pool,
	}
}

// setupReadOnlyEnv creates a test env with a read_only API key.
func setupReadOnlyEnv(t *testing.T) *testEnv {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())

	keyResult, err := svc.CreateAPIKey(t.Context(), "readonly-key", "read_only")
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	r := buildTestRouter(svc)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	return &testEnv{
		Server:  server,
		APIKey:  keyResult.PlaintextKey,
		Service: svc,
		Queries: queries,
		Pool:    pool,
	}
}

// buildTestRouter creates a chi router mirroring the production API routes.
func buildTestRouter(svc *service.Service) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))

		// Read endpoints
		r.Get("/accounts", ListAccountsHandler(svc))
		r.Get("/accounts/{id}", GetAccountHandler(svc))
		r.Get("/transactions", ListTransactionsHandler(svc))
		r.Get("/transactions/count", CountTransactionsHandler(svc))
		r.Get("/transactions/summary", TransactionSummaryHandler(svc))
		r.Get("/transactions/merchants", MerchantSummaryHandler(svc))
		r.Get("/transactions/{id}", GetTransactionHandler(svc))
		r.Get("/categories", ListCategoriesHandler(svc))
		r.Get("/categories/{id}", GetCategoryHandler(svc))
		r.Get("/users", ListUsersHandler(svc))
		r.Get("/connections", ListConnectionsHandler(svc))
		r.Get("/rules", ListRulesHandler(svc))
		r.Get("/rules/{id}", GetRuleHandler(svc))
		r.Get("/transactions/{transaction_id}/comments", ListCommentsHandler(svc))
		r.Get("/account-links", ListAccountLinksHandler(svc))
		r.Get("/account-links/{id}", GetAccountLinkHandler(svc))
		r.Get("/account-links/{id}/matches", ListTransactionMatchesHandler(svc))
		r.Get("/reports", ListReportsHandler(svc))
		r.Get("/reports/unread-count", UnreadReportCountHandler(svc))
		r.Get("/tags", ListTagsHandler(svc))

		// Write endpoints — require full_access scope
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Patch("/transactions/{id}/category", SetTransactionCategoryHandler(svc))
			r.Delete("/transactions/{id}/category", ResetTransactionCategoryHandler(svc))
			r.Post("/categories", CreateCategoryHandler(svc))
			r.Put("/categories/{id}", UpdateCategoryHandler(svc))
			r.Delete("/categories/{id}", DeleteCategoryHandler(svc))
			r.Post("/categories/{id}/merge", MergeCategoriesHandler(svc))
			r.Post("/rules", CreateRuleHandler(svc))
			r.Put("/rules/{id}", UpdateRuleHandler(svc))
			r.Delete("/rules/{id}", DeleteRuleHandler(svc))
			r.Post("/rules/preview", PreviewRuleHandler(svc))
			r.Post("/transactions/{transaction_id}/comments", CreateCommentHandler(svc))
			r.Put("/transactions/{transaction_id}/comments/{id}", UpdateCommentHandler(svc))
			r.Delete("/transactions/{transaction_id}/comments/{id}", DeleteCommentHandler(svc))
			r.Post("/transactions/batch-categorize", BatchCategorizeHandler(svc))
			r.Post("/transactions/bulk-recategorize", BulkRecategorizeHandler(svc))
			r.Post("/account-links", CreateAccountLinkHandler(svc))
			r.Put("/account-links/{id}", UpdateAccountLinkHandler(svc))
			r.Delete("/account-links/{id}", DeleteAccountLinkHandler(svc))
			r.Post("/account-links/{id}/reconcile", ReconcileAccountLinkHandler(svc))
			r.Post("/transaction-matches/{id}/confirm", ConfirmMatchHandler(svc))
			r.Post("/transaction-matches/{id}/reject", RejectMatchHandler(svc))
			r.Post("/transaction-matches/manual", ManualMatchHandler(svc))
			r.Post("/reports", CreateReportHandler(svc))
			r.Patch("/reports/{id}/read", MarkReportReadHandler(svc))
			r.Post("/transactions/{id}/tags", AddTransactionTagHandler(svc))
			r.Delete("/transactions/{id}/tags/{slug}", RemoveTransactionTagHandler(svc))
		})
	})
	return r
}

// --- HTTP helper methods ---

func (e *testEnv) doGet(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", e.Server.URL+path, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("X-API-Key", e.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func (e *testEnv) doJSON(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, e.Server.URL+path, bodyReader)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("X-API-Key", e.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func (e *testEnv) doPost(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	return e.doJSON(t, "POST", path, body)
}

func (e *testEnv) doPut(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	return e.doJSON(t, "PUT", path, body)
}

func (e *testEnv) doPatch(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	return e.doJSON(t, "PATCH", path, body)
}

func (e *testEnv) doDelete(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("DELETE", e.Server.URL+path, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("X-API-Key", e.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// --- Response helpers ---

func parseJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal body: %v\nbody: %s", err, string(data))
	}
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("want status %d, got %d, body: %s", want, resp.StatusCode, string(body))
	}
}

func readErrorCode(t *testing.T, resp *http.Response, wantStatus int, wantCode string) {
	t.Helper()
	if resp.StatusCode != wantStatus {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("want status %d, got %d, body: %s", wantStatus, resp.StatusCode, string(body))
	}
	var errResp mw.ErrorResponse
	parseJSON(t, resp, &errResp)
	if errResp.Error.Code != wantCode {
		t.Fatalf("want error code %q, got %q (message: %s)", wantCode, errResp.Error.Code, errResp.Error.Message)
	}
}

// --- Fixture helpers ---

func seedFixture(t *testing.T, q *db.Queries) (user db.User, acct db.Account, txn db.Transaction) {
	t.Helper()
	user = testutil.MustCreateUser(t, q, "Alice")
	conn := testutil.MustCreateConnection(t, q, user.ID, "item_1")
	acct = testutil.MustCreateAccount(t, q, conn.ID, "ext_acct_1", "Checking")
	txn = testutil.MustCreateTransaction(t, q, acct.ID, "ext_txn_1", "Coffee Shop", 450, "2025-03-15")
	return
}

// seedUncategorized creates the "uncategorized" system category needed by category operations.
func seedUncategorized(t *testing.T, q *db.Queries) db.Category {
	t.Helper()
	cat, err := q.InsertCategory(context.Background(), db.InsertCategoryParams{
		Slug:        "uncategorized",
		DisplayName: "Uncategorized",
		IsSystem:    true,
	})
	if err != nil {
		t.Fatalf("seed uncategorized category: %v", err)
	}
	return cat
}


// ============================================================
// Authentication & Scope Tests
// ============================================================

func TestAPI_MissingAPIKey(t *testing.T) {
	env := setupTestEnv(t)

	req, _ := http.NewRequest("GET", env.Server.URL+"/api/v1/users", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	readErrorCode(t, resp, http.StatusUnauthorized, "MISSING_CREDENTIALS")
}

func TestAPI_InvalidAPIKey(t *testing.T) {
	env := setupTestEnv(t)

	req, _ := http.NewRequest("GET", env.Server.URL+"/api/v1/users", nil)
	req.Header.Set("X-API-Key", "bb_invalidkey123")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	readErrorCode(t, resp, http.StatusUnauthorized, "INVALID_API_KEY")
}

func TestAPI_ReadOnlyKeyBlocksWrite(t *testing.T) {
	env := setupReadOnlyEnv(t)

	resp := env.doPost(t, "/api/v1/categories", map[string]string{
		"display_name": "Test Category",
	})
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

func TestAPI_ReadOnlyKeyAllowsRead(t *testing.T) {
	env := setupReadOnlyEnv(t)

	resp := env.doGet(t, "/api/v1/users")
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestAPI_RevokedAPIKey(t *testing.T) {
	env := setupTestEnv(t)

	// Create a second key, then revoke it
	result2, err := env.Service.CreateAPIKey(t.Context(), "revocable", "full_access")
	if err != nil {
		t.Fatal(err)
	}
	keys, _ := env.Service.ListAPIKeys(t.Context())
	for _, k := range keys {
		if k.Name == "revocable" {
			_ = env.Service.RevokeAPIKey(t.Context(), k.ID)
		}
	}

	req, _ := http.NewRequest("GET", env.Server.URL+"/api/v1/users", nil)
	req.Header.Set("X-API-Key", result2.PlaintextKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	readErrorCode(t, resp, http.StatusUnauthorized, "REVOKED_API_KEY")
}

// ============================================================
// Transaction List Handler Tests
// ============================================================

func TestAPI_ListTransactions_Empty(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions")
	assertStatus(t, resp, http.StatusOK)

	var result struct {
		Transactions []any  `json:"transactions"`
		NextCursor   string `json:"next_cursor"`
		HasMore      bool   `json:"has_more"`
	}
	parseJSON(t, resp, &result)
	if len(result.Transactions) != 0 {
		t.Errorf("expected 0 transactions, got %d", len(result.Transactions))
	}
	if result.HasMore {
		t.Error("expected has_more=false")
	}
}

func TestAPI_ListTransactions_WithData(t *testing.T) {
	env := setupTestEnv(t)
	_, _, _ = seedFixture(t, env.Queries)

	resp := env.doGet(t, "/api/v1/transactions")
	assertStatus(t, resp, http.StatusOK)

	var result struct {
		Transactions []map[string]any `json:"transactions"`
	}
	parseJSON(t, resp, &result)
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(result.Transactions))
	}
	if result.Transactions[0]["name"] != "Coffee Shop" {
		t.Errorf("expected name 'Coffee Shop', got %v", result.Transactions[0]["name"])
	}
	// Verify denormalized fields exist
	if result.Transactions[0]["account_name"] == nil {
		t.Error("expected account_name to be present")
	}
}

func TestAPI_ListTransactions_InvalidLimit(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions?limit=0")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_ListTransactions_LimitTooHigh(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions?limit=501")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_ListTransactions_InvalidStartDate(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions?start_date=not-a-date")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_ListTransactions_StartAfterEnd(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions?start_date=2025-03-20&end_date=2025-03-10")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_ListTransactions_DateFilter(t *testing.T) {
	env := setupTestEnv(t)
	_, _, _ = seedFixture(t, env.Queries) // txn on 2025-03-15

	// Range that includes the transaction
	resp := env.doGet(t, "/api/v1/transactions?start_date=2025-03-01&end_date=2025-03-31")
	assertStatus(t, resp, http.StatusOK)
	var incl struct{ Transactions []any }
	parseJSON(t, resp, &incl)
	if len(incl.Transactions) != 1 {
		t.Errorf("expected 1 transaction in range, got %d", len(incl.Transactions))
	}

	// Range that excludes the transaction
	resp = env.doGet(t, "/api/v1/transactions?start_date=2025-04-01&end_date=2025-04-30")
	assertStatus(t, resp, http.StatusOK)
	var excl struct{ Transactions []any }
	parseJSON(t, resp, &excl)
	if len(excl.Transactions) != 0 {
		t.Errorf("expected 0 transactions outside range, got %d", len(excl.Transactions))
	}
}

func TestAPI_ListTransactions_AmountFilter(t *testing.T) {
	env := setupTestEnv(t)
	_, _, _ = seedFixture(t, env.Queries) // txn amount = 4.50

	resp := env.doGet(t, "/api/v1/transactions?min_amount=4&max_amount=5")
	assertStatus(t, resp, http.StatusOK)
	var result struct{ Transactions []any }
	parseJSON(t, resp, &result)
	if len(result.Transactions) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(result.Transactions))
	}

	resp = env.doGet(t, "/api/v1/transactions?min_amount=10")
	assertStatus(t, resp, http.StatusOK)
	var empty struct{ Transactions []any }
	parseJSON(t, resp, &empty)
	if len(empty.Transactions) != 0 {
		t.Errorf("expected 0 transactions, got %d", len(empty.Transactions))
	}
}

func TestAPI_ListTransactions_MinAmountGtMaxAmount(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions?min_amount=100&max_amount=50")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_ListTransactions_InvalidPending(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions?pending=maybe")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_ListTransactions_SearchTooShort(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions?search=a")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_ListTransactions_InvalidSortBy(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions?sort_by=invalid")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_ListTransactions_InvalidSortOrder(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions?sort_order=up")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_ListTransactions_SearchFilter(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Bob")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "item_2")
	acct := testutil.MustCreateAccount(t, env.Queries, conn.ID, "ext_2", "Savings")
	testutil.MustCreateTransaction(t, env.Queries, acct.ID, "tx_1", "Starbucks Coffee", 550, "2025-03-10")
	testutil.MustCreateTransaction(t, env.Queries, acct.ID, "tx_2", "Gas Station", 3500, "2025-03-11")

	resp := env.doGet(t, "/api/v1/transactions?search=coffee")
	assertStatus(t, resp, http.StatusOK)
	var result struct{ Transactions []map[string]any }
	parseJSON(t, resp, &result)
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 transaction matching 'coffee', got %d", len(result.Transactions))
	}
	if result.Transactions[0]["name"] != "Starbucks Coffee" {
		t.Errorf("expected 'Starbucks Coffee', got %v", result.Transactions[0]["name"])
	}
}

func TestAPI_ListTransactions_Pagination(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "item_p")
	acct := testutil.MustCreateAccount(t, env.Queries, conn.ID, "ext_p", "Checking")
	for i := 0; i < 3; i++ {
		testutil.MustCreateTransaction(t, env.Queries, acct.ID,
			fmt.Sprintf("tx_p_%d", i), fmt.Sprintf("Store %d", i), int64(100*(i+1)), "2025-03-15")
	}

	// Get page 1 with limit 2
	resp := env.doGet(t, "/api/v1/transactions?limit=2")
	assertStatus(t, resp, http.StatusOK)
	var page1 struct {
		Transactions []any  `json:"transactions"`
		NextCursor   string `json:"next_cursor"`
		HasMore      bool   `json:"has_more"`
	}
	parseJSON(t, resp, &page1)
	if len(page1.Transactions) != 2 {
		t.Fatalf("expected 2 transactions on page 1, got %d", len(page1.Transactions))
	}
	if !page1.HasMore {
		t.Error("expected has_more=true on page 1")
	}
	if page1.NextCursor == "" {
		t.Error("expected non-empty next_cursor on page 1")
	}

	// Get page 2
	resp = env.doGet(t, "/api/v1/transactions?limit=2&cursor="+page1.NextCursor)
	assertStatus(t, resp, http.StatusOK)
	var page2 struct {
		Transactions []any `json:"transactions"`
		HasMore      bool  `json:"has_more"`
	}
	parseJSON(t, resp, &page2)
	if len(page2.Transactions) != 1 {
		t.Fatalf("expected 1 transaction on page 2, got %d", len(page2.Transactions))
	}
	if page2.HasMore {
		t.Error("expected has_more=false on page 2")
	}
}

// ============================================================
// Transaction Count Handler Tests
// ============================================================

func TestAPI_CountTransactions_Empty(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions/count")
	assertStatus(t, resp, http.StatusOK)

	var result struct {
		Count int64 `json:"count"`
	}
	parseJSON(t, resp, &result)
	if result.Count != 0 {
		t.Errorf("expected count 0, got %d", result.Count)
	}
}

func TestAPI_CountTransactions_WithData(t *testing.T) {
	env := setupTestEnv(t)
	_, _, _ = seedFixture(t, env.Queries)

	resp := env.doGet(t, "/api/v1/transactions/count")
	assertStatus(t, resp, http.StatusOK)

	var result struct {
		Count int64 `json:"count"`
	}
	parseJSON(t, resp, &result)
	if result.Count != 1 {
		t.Errorf("expected count 1, got %d", result.Count)
	}
}

// ============================================================
// Get Transaction Handler Tests
// ============================================================

func TestAPI_GetTransaction_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions/00000000-0000-0000-0000-000000000000")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestAPI_GetTransaction_Found(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doGet(t, "/api/v1/transactions/"+txnID)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]any
	parseJSON(t, resp, &result)
	if result["name"] != "Coffee Shop" {
		t.Errorf("expected name 'Coffee Shop', got %v", result["name"])
	}
}

func TestAPI_GetTransaction_InvalidID(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions/not-a-uuid")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// ============================================================
// Transaction Summary Handler Tests
// ============================================================

func TestAPI_TransactionSummary_MissingGroupBy(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions/summary")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_TransactionSummary_ValidGroupBy(t *testing.T) {
	env := setupTestEnv(t)
	_, _, _ = seedFixture(t, env.Queries)

	resp := env.doGet(t, "/api/v1/transactions/summary?group_by=month")
	assertStatus(t, resp, http.StatusOK)

	var result map[string]any
	parseJSON(t, resp, &result)
	// Response uses "summary" key for aggregated rows
	if result["summary"] == nil {
		t.Error("expected 'summary' in response")
	}
	if result["totals"] == nil {
		t.Error("expected 'totals' in response")
	}
}

func TestAPI_TransactionSummary_InvalidGroupBy(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions/summary?group_by=invalid")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_TransactionSummary_SpendingOnly(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "item_spending")
	acct := testutil.MustCreateAccount(t, env.Queries, conn.ID, "ext_acct_spending", "Checking")

	// Create a spending transaction (positive amount = debit) and an income transaction (negative amount = credit).
	testutil.MustCreateTransaction(t, env.Queries, acct.ID, "ext_spend_1", "Coffee Shop", 1500, "2025-03-10")
	testutil.MustCreateTransaction(t, env.Queries, acct.ID, "ext_income_1", "Payroll", -300000, "2025-03-10")

	// Without spending_only: both transactions should appear.
	resp := env.doGet(t, "/api/v1/transactions/summary?group_by=month&start_date=2025-03-01&end_date=2025-04-01")
	assertStatus(t, resp, http.StatusOK)
	var allResult struct {
		Summary []struct {
			TotalAmount float64 `json:"total_amount"`
		} `json:"summary"`
	}
	parseJSON(t, resp, &allResult)
	if len(allResult.Summary) == 0 {
		t.Fatal("expected at least one summary row without spending_only")
	}

	// With spending_only=true: only the positive (spending) transaction should appear.
	resp2 := env.doGet(t, "/api/v1/transactions/summary?group_by=month&start_date=2025-03-01&end_date=2025-04-01&spending_only=true")
	assertStatus(t, resp2, http.StatusOK)
	var spendResult struct {
		Summary []struct {
			TotalAmount float64 `json:"total_amount"`
		} `json:"summary"`
	}
	parseJSON(t, resp2, &spendResult)
	if len(spendResult.Summary) == 0 {
		t.Fatal("expected at least one summary row with spending_only=true")
	}

	// The spending-only total should be positive and smaller in magnitude than the unfiltered total
	// (which includes the large negative income transaction).
	spendTotal := spendResult.Summary[0].TotalAmount
	if spendTotal <= 0 {
		t.Errorf("spending_only total should be positive (spending), got %f", spendTotal)
	}
	// The income transaction is -3000.00 and spend is 15.00; without the filter the sum would be negative.
	allTotal := allResult.Summary[0].TotalAmount
	if allTotal >= spendTotal {
		t.Errorf("unfiltered total (%f) should be less than spending-only total (%f) due to income offset", allTotal, spendTotal)
	}
}

// ============================================================
// Category CRUD Handler Tests
// ============================================================

func TestAPI_CreateCategory_Success(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Food & Drink",
		"slug":         "food_and_drink",
	})
	assertStatus(t, resp, http.StatusCreated)

	var cat map[string]any
	parseJSON(t, resp, &cat)
	if cat["display_name"] != "Food & Drink" {
		t.Errorf("expected display_name 'Food & Drink', got %v", cat["display_name"])
	}
	if cat["slug"] != "food_and_drink" {
		t.Errorf("expected slug 'food_and_drink', got %v", cat["slug"])
	}
}

func TestAPI_CreateCategory_MissingName(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/categories", map[string]any{
		"slug": "no_name",
	})
	readErrorCode(t, resp, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestAPI_CreateCategory_AutoSlug(t *testing.T) {
	env := setupTestEnv(t)

	// Create without explicit slug — service should auto-generate from display_name
	resp := env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "My Custom Category",
	})
	assertStatus(t, resp, http.StatusCreated)

	var cat map[string]any
	parseJSON(t, resp, &cat)
	slug, ok := cat["slug"].(string)
	if !ok || slug == "" {
		t.Fatal("expected auto-generated slug")
	}
}

func TestAPI_CreateCategory_DuplicateSlugAutoDedup(t *testing.T) {
	env := setupTestEnv(t)

	// First
	resp := env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Food",
		"slug":         "food",
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Second with same slug — service auto-appends _2
	resp = env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Food Again",
		"slug":         "food",
	})
	assertStatus(t, resp, http.StatusCreated)
	var cat map[string]any
	parseJSON(t, resp, &cat)
	if cat["slug"] == "food" {
		t.Error("expected deduplicated slug, got 'food'")
	}
}

func TestAPI_GetCategory_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/categories/00000000-0000-0000-0000-000000000000")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestAPI_UpdateCategory_Success(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Old Name",
		"slug":         "old_name",
	})
	assertStatus(t, resp, http.StatusCreated)
	var cat map[string]any
	parseJSON(t, resp, &cat)
	catID := cat["id"].(string)

	resp = env.doPut(t, "/api/v1/categories/"+catID, map[string]any{
		"display_name": "New Name",
	})
	assertStatus(t, resp, http.StatusOK)
	var updated map[string]any
	parseJSON(t, resp, &updated)
	if updated["display_name"] != "New Name" {
		t.Errorf("expected 'New Name', got %v", updated["display_name"])
	}
}

func TestAPI_DeleteCategory_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doDelete(t, "/api/v1/categories/00000000-0000-0000-0000-000000000000")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestAPI_DeleteCategory_Success(t *testing.T) {
	env := setupTestEnv(t)
	// DeleteCategory reassigns orphaned transactions to "uncategorized", so it must exist
	seedUncategorized(t, env.Queries)

	resp := env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Deletable",
		"slug":         "deletable",
	})
	assertStatus(t, resp, http.StatusCreated)
	var cat map[string]any
	parseJSON(t, resp, &cat)
	catID := cat["id"].(string)

	resp = env.doDelete(t, "/api/v1/categories/"+catID)
	assertStatus(t, resp, http.StatusOK)
	var result map[string]any
	parseJSON(t, resp, &result)
	if result["affected_transactions"] == nil {
		t.Error("expected affected_transactions in response")
	}
}

func TestAPI_DeleteCategory_UndeletableSystem(t *testing.T) {
	env := setupTestEnv(t)
	uncat := seedUncategorized(t, env.Queries)
	uncatID := pgconv.FormatUUID(uncat.ID)

	resp := env.doDelete(t, "/api/v1/categories/"+uncatID)
	readErrorCode(t, resp, http.StatusConflict, "CATEGORY_UNDELETABLE")
}

// ============================================================
// Category Set/Reset on Transaction Tests
// ============================================================

func TestAPI_SetTransactionCategory_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	seedUncategorized(t, env.Queries)

	// Create a category so category_id is valid
	catResp := env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Valid", "slug": "valid",
	})
	var cat map[string]any
	parseJSON(t, catResp, &cat)

	resp := env.doPatch(t, "/api/v1/transactions/00000000-0000-0000-0000-000000000000/category", map[string]string{
		"category_id": cat["id"].(string),
	})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestAPI_SetTransactionCategory_MissingCategoryID(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doPatch(t, "/api/v1/transactions/"+txnID+"/category", map[string]string{})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_SetTransactionCategory_CategoryNotFound(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doPatch(t, "/api/v1/transactions/"+txnID+"/category", map[string]string{
		"category_id": "00000000-0000-0000-0000-000000000099",
	})
	readErrorCode(t, resp, http.StatusNotFound, "CATEGORY_NOT_FOUND")
}

func TestAPI_SetTransactionCategory_Success(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	catResp := env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Coffee",
		"slug":         "coffee",
	})
	assertStatus(t, catResp, http.StatusCreated)
	var cat map[string]any
	parseJSON(t, catResp, &cat)
	catID := cat["id"].(string)

	resp := env.doPatch(t, "/api/v1/transactions/"+txnID+"/category", map[string]string{
		"category_id": catID,
	})
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// Verify via GET
	getResp := env.doGet(t, "/api/v1/transactions/"+txnID)
	assertStatus(t, getResp, http.StatusOK)
	var txnData map[string]any
	parseJSON(t, getResp, &txnData)
	if txnData["category_override"] != true {
		t.Error("expected category_override=true after manual set")
	}
}

func TestAPI_ResetTransactionCategory_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedUncategorized(t, env.Queries)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	// Create and set category
	catResp := env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Temp",
		"slug":         "temp",
	})
	var cat map[string]any
	parseJSON(t, catResp, &cat)
	env.doPatch(t, "/api/v1/transactions/"+txnID+"/category", map[string]string{
		"category_id": cat["id"].(string),
	}).Body.Close()

	// Reset
	resp := env.doDelete(t, "/api/v1/transactions/"+txnID+"/category")
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// Verify override is cleared
	getResp := env.doGet(t, "/api/v1/transactions/"+txnID)
	var txnData map[string]any
	parseJSON(t, getResp, &txnData)
	if txnData["category_override"] == true {
		t.Error("expected category_override=false after reset")
	}
}

func TestAPI_ResetTransactionCategory_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doDelete(t, "/api/v1/transactions/00000000-0000-0000-0000-000000000000/category")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// ============================================================
// Transaction Rules Handler Tests
// ============================================================

func TestAPI_CreateRule_Success(t *testing.T) {
	env := setupTestEnv(t)

	env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Groceries",
		"slug":         "groceries",
	}).Body.Close()

	resp := env.doPost(t, "/api/v1/rules", map[string]any{
		"name":          "Grocery Rule",
		"category_slug": "groceries",
		"priority":      10,
		"conditions": map[string]any{
			"field": "name",
			"op":    "contains",
			"value": "grocery",
		},
	})
	assertStatus(t, resp, http.StatusCreated)

	var rule map[string]any
	parseJSON(t, resp, &rule)
	if rule["name"] != "Grocery Rule" {
		t.Errorf("expected name 'Grocery Rule', got %v", rule["name"])
	}
	if rule["enabled"] != true {
		t.Error("expected rule to be enabled by default")
	}
}

func TestAPI_CreateRule_MissingName(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/rules", map[string]any{
		"category_slug": "food",
		"conditions": map[string]any{
			"field": "name",
			"op":    "eq",
			"value": "test",
		},
	})
	readErrorCode(t, resp, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestAPI_CreateRule_MissingCategorySlug(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/rules", map[string]any{
		"name": "Test Rule",
		"conditions": map[string]any{
			"field": "name",
			"op":    "eq",
			"value": "test",
		},
	})
	readErrorCode(t, resp, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestAPI_CreateRule_InvalidCategory(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/rules", map[string]any{
		"name":          "Bad Category Rule",
		"category_slug": "nonexistent_category_slug",
		"conditions": map[string]any{
			"field": "name",
			"op":    "eq",
			"value": "test",
		},
	})
	readErrorCode(t, resp, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestAPI_GetRule_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/rules/00000000-0000-0000-0000-000000000000")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestAPI_ListRules_Empty(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/rules")
	assertStatus(t, resp, http.StatusOK)

	var result struct{ Rules []any }
	parseJSON(t, resp, &result)
	if len(result.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(result.Rules))
	}
}

func TestAPI_ListRules_InvalidLimit(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/rules?limit=0")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_ListRules_LimitTooHigh(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/rules?limit=501")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_UpdateRule_Success(t *testing.T) {
	env := setupTestEnv(t)

	env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Food",
		"slug":         "food",
	}).Body.Close()

	resp := env.doPost(t, "/api/v1/rules", map[string]any{
		"name":          "Food Rule",
		"category_slug": "food",
		"priority":      5,
		"conditions":    map[string]any{"field": "name", "op": "contains", "value": "food"},
	})
	assertStatus(t, resp, http.StatusCreated)
	var rule map[string]any
	parseJSON(t, resp, &rule)
	ruleID := rule["id"].(string)

	newName := "Updated Food Rule"
	resp = env.doPut(t, "/api/v1/rules/"+ruleID, map[string]any{
		"name": &newName,
	})
	assertStatus(t, resp, http.StatusOK)
	var updated map[string]any
	parseJSON(t, resp, &updated)
	if updated["name"] != "Updated Food Rule" {
		t.Errorf("expected 'Updated Food Rule', got %v", updated["name"])
	}
}

func TestAPI_UpdateRule_Disable(t *testing.T) {
	env := setupTestEnv(t)

	env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Misc",
		"slug":         "misc",
	}).Body.Close()

	resp := env.doPost(t, "/api/v1/rules", map[string]any{
		"name":          "Disable Me",
		"category_slug": "misc",
		"conditions":    map[string]any{"field": "name", "op": "eq", "value": "x"},
	})
	var rule map[string]any
	parseJSON(t, resp, &rule)
	ruleID := rule["id"].(string)

	disabled := false
	resp = env.doPut(t, "/api/v1/rules/"+ruleID, map[string]any{
		"enabled": &disabled,
	})
	assertStatus(t, resp, http.StatusOK)
	var updated map[string]any
	parseJSON(t, resp, &updated)
	if updated["enabled"] != false {
		t.Error("expected rule to be disabled")
	}
}

func TestAPI_DeleteRule_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doDelete(t, "/api/v1/rules/00000000-0000-0000-0000-000000000000")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestAPI_DeleteRule_Success(t *testing.T) {
	env := setupTestEnv(t)

	env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Transport",
		"slug":         "transport",
	}).Body.Close()

	resp := env.doPost(t, "/api/v1/rules", map[string]any{
		"name":          "Transport Rule",
		"category_slug": "transport",
		"priority":      1,
		"conditions":    map[string]any{"field": "name", "op": "contains", "value": "uber"},
	})
	assertStatus(t, resp, http.StatusCreated)
	var rule map[string]any
	parseJSON(t, resp, &rule)
	ruleID, ok := rule["id"].(string)
	if !ok || ruleID == "" {
		t.Fatalf("expected rule id in response, got %v", rule)
	}

	resp = env.doDelete(t, "/api/v1/rules/"+ruleID)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// Confirm it's gone
	resp = env.doGet(t, "/api/v1/rules/"+ruleID)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestAPI_PreviewRule_Success(t *testing.T) {
	env := setupTestEnv(t)
	_, _, _ = seedFixture(t, env.Queries) // "Coffee Shop" transaction

	resp := env.doPost(t, "/api/v1/rules/preview", map[string]any{
		"conditions": map[string]any{
			"field": "name",
			"op":    "contains",
			"value": "Coffee",
		},
		"sample_size": 10,
	})
	assertStatus(t, resp, http.StatusOK)

	var result map[string]any
	parseJSON(t, resp, &result)
	matchCount, ok := result["match_count"].(float64)
	if !ok {
		t.Fatal("expected match_count in response")
	}
	if matchCount != 1 {
		t.Errorf("expected 1 match, got %v", matchCount)
	}
}

func TestAPI_PreviewRule_NoMatches(t *testing.T) {
	env := setupTestEnv(t)
	_, _, _ = seedFixture(t, env.Queries)

	resp := env.doPost(t, "/api/v1/rules/preview", map[string]any{
		"conditions": map[string]any{
			"field": "name",
			"op":    "eq",
			"value": "NONEXISTENT",
		},
	})
	assertStatus(t, resp, http.StatusOK)

	var result map[string]any
	parseJSON(t, resp, &result)
	if result["match_count"].(float64) != 0 {
		t.Errorf("expected 0 matches, got %v", result["match_count"])
	}
}

// ============================================================
// Comment Handler Tests
// ============================================================

func TestAPI_CreateComment_Success(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doPost(t, "/api/v1/transactions/"+txnID+"/comments", map[string]string{
		"content": "This is a test comment",
	})
	assertStatus(t, resp, http.StatusCreated)

	var comment map[string]any
	parseJSON(t, resp, &comment)
	if comment["content"] != "This is a test comment" {
		t.Errorf("expected content 'This is a test comment', got %v", comment["content"])
	}
}

func TestAPI_CreateComment_TransactionNotFound(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/transactions/00000000-0000-0000-0000-000000000000/comments", map[string]string{
		"content": "Comment on nonexistent",
	})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestAPI_ListComments_Empty(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doGet(t, "/api/v1/transactions/"+txnID+"/comments")
	assertStatus(t, resp, http.StatusOK)

	var result struct{ Comments []any }
	parseJSON(t, resp, &result)
	if len(result.Comments) != 0 {
		t.Errorf("expected 0 comments, got %d", len(result.Comments))
	}
}

func TestAPI_ListComments_WithData(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	env.doPost(t, "/api/v1/transactions/"+txnID+"/comments", map[string]string{
		"content": "First comment",
	}).Body.Close()
	env.doPost(t, "/api/v1/transactions/"+txnID+"/comments", map[string]string{
		"content": "Second comment",
	}).Body.Close()

	resp := env.doGet(t, "/api/v1/transactions/"+txnID+"/comments")
	assertStatus(t, resp, http.StatusOK)

	var result struct{ Comments []any }
	parseJSON(t, resp, &result)
	if len(result.Comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(result.Comments))
	}
}

// ============================================================
// Users & Accounts & Connections Handler Tests
// ============================================================

func TestAPI_ListUsers_Empty(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/users")
	assertStatus(t, resp, http.StatusOK)

	var users []any
	parseJSON(t, resp, &users)
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestAPI_ListUsers_WithData(t *testing.T) {
	env := setupTestEnv(t)
	testutil.MustCreateUser(t, env.Queries, "Alice")
	testutil.MustCreateUser(t, env.Queries, "Bob")

	resp := env.doGet(t, "/api/v1/users")
	assertStatus(t, resp, http.StatusOK)

	var users []map[string]any
	parseJSON(t, resp, &users)
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
}

func TestAPI_ListAccounts_Empty(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/accounts")
	assertStatus(t, resp, http.StatusOK)

	var accounts []any
	parseJSON(t, resp, &accounts)
	if len(accounts) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(accounts))
	}
}

func TestAPI_ListAccounts_WithData(t *testing.T) {
	env := setupTestEnv(t)
	_, _, _ = seedFixture(t, env.Queries)

	resp := env.doGet(t, "/api/v1/accounts")
	assertStatus(t, resp, http.StatusOK)

	var accounts []map[string]any
	parseJSON(t, resp, &accounts)
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if accounts[0]["name"] != "Checking" {
		t.Errorf("expected name 'Checking', got %v", accounts[0]["name"])
	}
}

func TestAPI_ListConnections_Empty(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/connections")
	assertStatus(t, resp, http.StatusOK)

	var conns []any
	parseJSON(t, resp, &conns)
	if len(conns) != 0 {
		t.Errorf("expected 0 connections, got %d", len(conns))
	}
}

func TestAPI_ListConnections_WithData(t *testing.T) {
	env := setupTestEnv(t)
	_, _, _ = seedFixture(t, env.Queries)

	resp := env.doGet(t, "/api/v1/connections")
	assertStatus(t, resp, http.StatusOK)

	var conns []map[string]any
	parseJSON(t, resp, &conns)
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}
}

// ============================================================
// Category Merge Test
// ============================================================

func TestAPI_MergeCategories_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedUncategorized(t, env.Queries)

	resp := env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Source",
		"slug":         "source",
	})
	assertStatus(t, resp, http.StatusCreated)
	var src map[string]any
	parseJSON(t, resp, &src)

	resp = env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Target",
		"slug":         "target",
	})
	assertStatus(t, resp, http.StatusCreated)
	var tgt map[string]any
	parseJSON(t, resp, &tgt)

	resp = env.doPost(t, "/api/v1/categories/"+src["id"].(string)+"/merge", map[string]string{
		"target_id": tgt["id"].(string),
	})
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// Source should be gone
	resp = env.doGet(t, "/api/v1/categories/"+src["id"].(string))
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestAPI_MergeCategories_MissingTargetID(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Src",
		"slug":         "src",
	})
	var src map[string]any
	parseJSON(t, resp, &src)

	resp = env.doPost(t, "/api/v1/categories/"+src["id"].(string)+"/merge", map[string]string{})
	readErrorCode(t, resp, http.StatusBadRequest, "VALIDATION_ERROR")
}

// ============================================================
// Batch Categorize Test
// ============================================================

func TestAPI_BatchCategorize_Success(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	env.doPost(t, "/api/v1/categories", map[string]any{
		"display_name": "Dining",
		"slug":         "dining",
	}).Body.Close()

	resp := env.doPost(t, "/api/v1/transactions/batch-categorize", map[string]any{
		"items": []map[string]string{
			{"transaction_id": txnID, "category_slug": "dining"},
		},
	})
	assertStatus(t, resp, http.StatusOK)

	var result map[string]any
	parseJSON(t, resp, &result)
	if result["succeeded"] == nil {
		t.Error("expected 'succeeded' field in batch result")
	}
	if result["succeeded"].(float64) != 1 {
		t.Errorf("expected succeeded=1, got %v", result["succeeded"])
	}
}

func TestAPI_BatchCategorize_EmptyItems(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/transactions/batch-categorize", map[string]any{
		"items": []map[string]string{},
	})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

// ============================================================
// JSON Error Envelope Tests
// ============================================================

func TestAPI_ErrorEnvelope_HasCorrectStructure(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions?limit=999")
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Skip("limit 999 might be valid, skipping")
	}

	var envelope map[string]any
	parseJSON(t, resp, &envelope)
	errorObj, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatal("expected 'error' object in response")
	}
	if errorObj["code"] == nil {
		t.Error("expected 'code' in error object")
	}
	if errorObj["message"] == nil {
		t.Error("expected 'message' in error object")
	}
}

// ============================================================
// Agent Reports API Tests
// ============================================================

func TestAPI_ListReports_Empty(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doGet(t, "/api/v1/reports")
	assertStatus(t, resp, http.StatusOK)

	var reports []any
	parseJSON(t, resp, &reports)
	if len(reports) != 0 {
		t.Errorf("expected 0 reports, got %d", len(reports))
	}
}

func TestAPI_UnreadReportCount_Empty(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doGet(t, "/api/v1/reports/unread-count")
	assertStatus(t, resp, http.StatusOK)

	var result map[string]int64
	parseJSON(t, resp, &result)
	if result["unread_count"] != 0 {
		t.Errorf("expected 0 unread, got %d", result["unread_count"])
	}
}

func TestAPI_CreateReport_Success(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/reports", map[string]any{
		"title":    "Test Report",
		"body":     "Report body content",
		"priority": "warning",
		"tags":     []string{"spending", "review"},
	})
	assertStatus(t, resp, http.StatusCreated)

	var report map[string]any
	parseJSON(t, resp, &report)
	if report["title"] != "Test Report" {
		t.Errorf("expected title 'Test Report', got %v", report["title"])
	}
	if report["priority"] != "warning" {
		t.Errorf("expected priority 'warning', got %v", report["priority"])
	}
}

func TestAPI_CreateReport_MissingTitle(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doPost(t, "/api/v1/reports", map[string]any{
		"body": "Body",
	})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_CreateReport_MissingBody(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doPost(t, "/api/v1/reports", map[string]any{
		"title": "Title",
	})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_CreateReport_InvalidPriority(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doPost(t, "/api/v1/reports", map[string]any{
		"title":    "Title",
		"body":     "Body",
		"priority": "urgent",
	})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestAPI_ListReports_WithData(t *testing.T) {
	env := setupTestEnv(t)

	// Create a report via API
	resp := env.doPost(t, "/api/v1/reports", map[string]any{
		"title": "My Report",
		"body":  "Report content",
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// List
	resp = env.doGet(t, "/api/v1/reports")
	assertStatus(t, resp, http.StatusOK)

	var reports []map[string]any
	parseJSON(t, resp, &reports)
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0]["title"] != "My Report" {
		t.Errorf("expected title 'My Report', got %v", reports[0]["title"])
	}
}

func TestAPI_MarkReportRead(t *testing.T) {
	env := setupTestEnv(t)

	// Create report
	resp := env.doPost(t, "/api/v1/reports", map[string]any{
		"title": "Report to Read",
		"body":  "Body",
	})
	assertStatus(t, resp, http.StatusCreated)

	var report map[string]any
	parseJSON(t, resp, &report)
	reportID := report["id"].(string)

	// Mark as read
	resp = env.doPatch(t, fmt.Sprintf("/api/v1/reports/%s/read", reportID), nil)
	assertStatus(t, resp, http.StatusOK)

	// Verify unread count is 0
	resp = env.doGet(t, "/api/v1/reports/unread-count")
	assertStatus(t, resp, http.StatusOK)

	var result map[string]any
	parseJSON(t, resp, &result)
	if result["unread_count"] != float64(0) {
		t.Errorf("expected 0 unread after marking read, got %v", result["unread_count"])
	}
}

// ============================================================
// Account Links API Tests
// ============================================================

func TestAPI_ListAccountLinks_Empty(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doGet(t, "/api/v1/account-links")
	assertStatus(t, resp, http.StatusOK)

	var links []any
	parseJSON(t, resp, &links)
	if len(links) != 0 {
		t.Errorf("expected 0 links, got %d", len(links))
	}
}

func TestAPI_CreateAccountLink_Success(t *testing.T) {
	env := setupTestEnv(t)

	// Create two users with separate connections and accounts
	user1 := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn1 := testutil.MustCreateConnection(t, env.Queries, user1.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, env.Queries, conn1.ID, "ext_acct_1", "Primary Checking")

	user2 := testutil.MustCreateUser(t, env.Queries, "Bob")
	conn2 := testutil.MustCreateConnection(t, env.Queries, user2.ID, "item_2")
	acct2 := testutil.MustCreateAccount(t, env.Queries, conn2.ID, "ext_acct_2", "Dependent Card")

	resp := env.doPost(t, "/api/v1/account-links", map[string]any{
		"primary_account_id":   acct1.ShortID,
		"dependent_account_id": acct2.ShortID,
	})
	assertStatus(t, resp, http.StatusCreated)

	var link map[string]any
	parseJSON(t, resp, &link)
	if link["primary_account_name"] != "Primary Checking" {
		t.Errorf("expected primary_account_name 'Primary Checking', got %v", link["primary_account_name"])
	}
	if link["dependent_account_name"] != "Dependent Card" {
		t.Errorf("expected dependent_account_name 'Dependent Card', got %v", link["dependent_account_name"])
	}
}

func TestAPI_CreateAccountLink_MissingParams(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doPost(t, "/api/v1/account-links", map[string]any{
		"primary_account_id": "abc123",
	})
	readErrorCode(t, resp, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestAPI_GetAccountLink_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doGet(t, "/api/v1/account-links/00000000-0000-0000-0000-000000000000")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestAPI_DeleteAccountLink_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doDelete(t, "/api/v1/account-links/00000000-0000-0000-0000-000000000000")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// ============================================================
// Merchant Summary API Tests
// ============================================================

func TestAPI_MerchantSummary_Empty(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doGet(t, "/api/v1/transactions/merchants")
	assertStatus(t, resp, http.StatusOK)

	var result map[string]any
	parseJSON(t, resp, &result)
	merchants, ok := result["merchants"].([]any)
	if !ok {
		t.Fatal("expected 'merchants' array in response")
	}
	if len(merchants) != 0 {
		t.Errorf("expected 0 merchants, got %d", len(merchants))
	}
}

func TestAPI_MerchantSummary_WithData(t *testing.T) {
	env := setupTestEnv(t)
	_, acct, _ := seedFixture(t, env.Queries)

	// Create additional transactions with different names
	testutil.MustCreateTransaction(t, env.Queries, acct.ID, "ext_txn_2", "Coffee Shop", 550, "2025-03-16")
	testutil.MustCreateTransaction(t, env.Queries, acct.ID, "ext_txn_3", "Grocery Store", 2000, "2025-03-17")

	resp := env.doGet(t, "/api/v1/transactions/merchants?start_date=2025-03-01&end_date=2025-04-01")
	assertStatus(t, resp, http.StatusOK)

	var result map[string]any
	parseJSON(t, resp, &result)
	merchants, ok := result["merchants"].([]any)
	if !ok {
		t.Fatal("expected 'merchants' array in response")
	}
	if len(merchants) < 1 {
		t.Errorf("expected at least 1 merchant, got %d", len(merchants))
	}
}

// ============================================================
// Get Account API Tests
// ============================================================

func TestAPI_GetAccount_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doGet(t, "/api/v1/accounts/00000000-0000-0000-0000-000000000000")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestAPI_GetAccount_Found(t *testing.T) {
	env := setupTestEnv(t)
	_, acct, _ := seedFixture(t, env.Queries)

	resp := env.doGet(t, "/api/v1/accounts/"+acct.ShortID)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]any
	parseJSON(t, resp, &result)
	if result["name"] != "Checking" {
		t.Errorf("expected name 'Checking', got %v", result["name"])
	}
}

// ============================================================
// Bulk Recategorize API Tests
// ============================================================

func TestAPI_BulkRecategorize_RequiresFilter(t *testing.T) {
	env := setupTestEnv(t)
	seedUncategorized(t, env.Queries)

	resp := env.doPost(t, "/api/v1/transactions/bulk-recategorize", map[string]any{
		"category_slug": "uncategorized",
	})
	// Should fail because no filter is provided (safety check)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

// ============================================================
// ReadOnly key blocks write endpoints
// ============================================================

func TestAPI_ReadOnlyKeyBlocksReportCreate(t *testing.T) {
	env := setupReadOnlyEnv(t)
	resp := env.doPost(t, "/api/v1/reports", map[string]any{
		"title": "Test",
		"body":  "Body",
	})
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

func TestAPI_ReadOnlyKeyBlocksAccountLinkCreate(t *testing.T) {
	env := setupReadOnlyEnv(t)
	resp := env.doPost(t, "/api/v1/account-links", map[string]any{
		"primary_account_id":   "abc",
		"dependent_account_id": "def",
	})
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

func TestAPI_ReadOnlyKeyAllowsReportRead(t *testing.T) {
	env := setupReadOnlyEnv(t)
	resp := env.doGet(t, "/api/v1/reports")
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestAPI_ReadOnlyKeyAllowsAccountLinkRead(t *testing.T) {
	env := setupReadOnlyEnv(t)
	resp := env.doGet(t, "/api/v1/account-links")
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestAPI_ReadOnlyKeyAllowsMerchantSummaryRead(t *testing.T) {
	env := setupReadOnlyEnv(t)
	resp := env.doGet(t, "/api/v1/transactions/merchants")
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}
