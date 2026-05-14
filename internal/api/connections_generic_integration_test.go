//go:build integration && !lite

// Integration tests for the generic POST /api/v1/connections endpoint plus
// the backward-compat shims (POST /connections/plaid/exchange,
// POST /connections/teller, POST /connections/csv/import).
//
// Reuses fakePlaidProvider + fakeTellerProvider from the per-provider test
// files in this package.
package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"breadbox/internal/app"
	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	"breadbox/internal/provider"
	"breadbox/internal/service"
	bsync "breadbox/internal/sync"
	"breadbox/internal/testutil"

	"github.com/go-chi/chi/v5"
)

// genericCreateEnv is the per-test harness for POST /connections. Both
// Plaid and Teller fakes are registered so the dispatch table can reach them.
type genericCreateEnv struct {
	Server  *httptest.Server
	APIKey  string
	App     *app.App
	Service *service.Service
	Queries *db.Queries
	Plaid   *fakePlaidProvider
	Teller  *fakeTellerProvider
}

func setupGenericCreateEnv(t *testing.T, scope string) *genericCreateEnv {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	engine := bsync.NewEngine(queries, pool, nil, slog.Default())
	svc := service.New(queries, pool, engine, slog.Default())

	keyResult, err := svc.CreateAPIKeyLegacy(t.Context(), "generic-create-test-key", scope)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	fp := &fakePlaidProvider{
		exchangeConn: provider.Connection{
			ProviderName:         "plaid",
			ExternalID:           "ext_new_item_456",
			EncryptedCredentials: []byte("encrypted-access-token"),
		},
		exchangeAccounts: []provider.Account{
			{ExternalID: "ext_acct_chk_1", Name: "Plaid Checking", Type: "depository", Subtype: "checking", Mask: "0000", ISOCurrencyCode: "USD"},
		},
	}
	ft := &fakeTellerProvider{
		exchangeConn: provider.Connection{
			ProviderName:         "teller",
			ExternalID:           "enr_xyz_888",
			EncryptedCredentials: []byte("encrypted-teller-token"),
			InstitutionName:      "Chase",
		},
		exchangeAccounts: []provider.Account{
			{ExternalID: "acc_chk_1", Name: "Chase Checking", Type: "depository", Subtype: "checking", Mask: "1234", ISOCurrencyCode: "USD"},
		},
	}

	a := &app.App{
		DB:        pool,
		Queries:   queries,
		Logger:    slog.Default(),
		Service:   svc,
		Providers: map[string]provider.Provider{"plaid": fp, "teller": ft},
	}

	r := buildGenericCreateTestRouter(svc, a)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	return &genericCreateEnv{
		Server:  server,
		APIKey:  keyResult.PlaintextKey,
		App:     a,
		Service: svc,
		Queries: queries,
		Plaid:   fp,
		Teller:  ft,
	}
}

// buildGenericCreateTestRouter wires the new generic routes plus the legacy
// per-provider routes that this test file asserts backward compatibility for.
func buildGenericCreateTestRouter(svc *service.Service, a *app.App) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Post("/connections", CreateConnectionHandler(a))
			// Backward-compat: keep the per-provider routes wired so we can
			// assert they still return the canonical envelope.
			r.Post("/connections/plaid/exchange", PlaidExchangeHandler(a))
			r.Post("/connections/teller", TellerSetupHandler(a))
			r.Post("/connections/csv/import", CSVImportHandler(svc))
		})
	})
	return r
}

func (e *genericCreateEnv) doPostJSON(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest("POST", e.Server.URL+path, &buf)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", e.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func (e *genericCreateEnv) doPostMultipart(t *testing.T, path string, body io.Reader, contentType string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("POST", e.Server.URL+path, body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-API-Key", e.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// ---- Plaid via generic endpoint ----

func TestCreateConnection_Plaid_Success(t *testing.T) {
	env := setupGenericCreateEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	body := map[string]any{
		"provider": "plaid",
		"user_id":  pgconv.FormatUUID(user.ID),
		"credentials": map[string]any{
			"public_token":     "public-sandbox-fake-public-token",
			"institution_id":   "ins_109511",
			"institution_name": "Chase",
		},
	}
	resp := env.doPostJSON(t, "/api/v1/connections", body)
	assertStatus(t, resp, http.StatusCreated)

	var out connectionEnvelope
	parseJSON(t, resp, &out)
	if out.ConnectionID == "" {
		t.Fatal("want non-empty connection_id")
	}
	if out.InstitutionName != "Chase" {
		t.Errorf("want institution_name=Chase, got %q", out.InstitutionName)
	}
	if out.Status != "active" {
		t.Errorf("want status=active, got %q", out.Status)
	}
	if env.Plaid.exchangeCalls != 1 {
		t.Errorf("want 1 exchange call on plaid provider, got %d", env.Plaid.exchangeCalls)
	}

	// Verify DB persistence: same shape as the legacy exchange handler.
	uid, err := env.Service.ResolveConnectionUUID(t.Context(), out.ConnectionID)
	if err != nil {
		t.Fatalf("resolve persisted connection: %v", err)
	}
	conn, err := env.Queries.GetBankConnection(t.Context(), uid)
	if err != nil {
		t.Fatalf("reload connection: %v", err)
	}
	if conn.Provider != db.ProviderTypePlaid {
		t.Errorf("want provider=plaid, got %q", conn.Provider)
	}
	if conn.ExternalID.String != "ext_new_item_456" {
		t.Errorf("want external_id from provider, got %q", conn.ExternalID.String)
	}
}

func TestCreateConnection_Teller_Success(t *testing.T) {
	env := setupGenericCreateEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	body := map[string]any{
		"provider": "teller",
		"user_id":  pgconv.FormatUUID(user.ID),
		"credentials": map[string]any{
			"access_token":     "token-sandbox-fake-teller",
			"enrollment_id":    "enr_abc123",
			"institution_id":   "ins_chase",
			"institution_name": "Chase",
		},
	}
	resp := env.doPostJSON(t, "/api/v1/connections", body)
	assertStatus(t, resp, http.StatusCreated)

	var out connectionEnvelope
	parseJSON(t, resp, &out)
	if out.ConnectionID == "" {
		t.Fatal("want non-empty connection_id")
	}
	if env.Teller.exchangeCalls != 1 {
		t.Errorf("want 1 exchange call on teller provider, got %d", env.Teller.exchangeCalls)
	}

	// The provider sees the JSON-encoded blob, identical to legacy path.
	var seen tellerExchangeBlob
	if err := json.Unmarshal([]byte(env.Teller.lastExchangeToken), &seen); err != nil {
		t.Fatalf("provider received non-JSON token: %v", err)
	}
	if seen.AccessToken != "token-sandbox-fake-teller" {
		t.Errorf("want access_token forwarded, got %q", seen.AccessToken)
	}
}

func TestCreateConnection_CSV_JSON_Success(t *testing.T) {
	env := setupGenericCreateEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	seedUncategorized(t, env.Queries)

	body := map[string]any{
		"provider": "csv",
		"user_id":  pgconv.FormatUUID(user.ID),
		"credentials": map[string]any{
			"csv_base64":     base64.StdEncoding.EncodeToString([]byte(sampleCSV)),
			"column_mapping": sampleCSVColumnMapping(),
			"account_name":   "Generic CSV",
			"date_format":    "2006-01-02",
		},
	}
	resp := env.doPostJSON(t, "/api/v1/connections", body)
	assertStatus(t, resp, http.StatusCreated)

	var out csvImportResponse
	parseJSON(t, resp, &out)
	if out.ConnectionID == "" {
		t.Fatal("want non-empty connection_id")
	}
	if out.ImportedTransactions != 2 {
		t.Errorf("want 2 imported, got %d", out.ImportedTransactions)
	}
}

func TestCreateConnection_CSV_Multipart_Success(t *testing.T) {
	env := setupGenericCreateEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	seedUncategorized(t, env.Queries)

	// Build a multipart body with `provider=csv`, the canonical CSV form
	// fields the legacy handler accepts, and the resolved user_id.
	var buf bytes.Buffer
	mwriter := multipart.NewWriter(&buf)
	part, err := mwriter.CreateFormFile("file", "sample.csv")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(part, sampleCSV); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	_ = mwriter.WriteField("provider", "csv")
	_ = mwriter.WriteField("user_id", pgconv.FormatUUID(user.ID))
	_ = mwriter.WriteField("account_name", "Multipart CSV")
	_ = mwriter.WriteField("date_format", "2006-01-02")
	mappingJSON, _ := json.Marshal(sampleCSVColumnMapping())
	_ = mwriter.WriteField("column_mapping", string(mappingJSON))
	if err := mwriter.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	resp := env.doPostMultipart(t, "/api/v1/connections", &buf, mwriter.FormDataContentType())
	assertStatus(t, resp, http.StatusCreated)

	var out csvImportResponse
	parseJSON(t, resp, &out)
	if out.ConnectionID == "" {
		t.Fatal("want non-empty connection_id")
	}
	if out.ImportedTransactions != 2 {
		t.Errorf("want 2 imported, got %d", out.ImportedTransactions)
	}
}

func TestCreateConnection_UnknownProvider(t *testing.T) {
	env := setupGenericCreateEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	body := map[string]any{
		"provider":    "moneykit",
		"user_id":     pgconv.FormatUUID(user.ID),
		"credentials": map[string]any{},
	}
	resp := env.doPostJSON(t, "/api/v1/connections", body)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestCreateConnection_MissingRequiredCredentials(t *testing.T) {
	env := setupGenericCreateEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	body := map[string]any{
		"provider": "plaid",
		"user_id":  pgconv.FormatUUID(user.ID),
		"credentials": map[string]any{
			"public_token": "public-sandbox-fake",
			// missing institution_id and institution_name
		},
	}
	resp := env.doPostJSON(t, "/api/v1/connections", body)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
	if env.Plaid.exchangeCalls != 0 {
		t.Errorf("provider should not be called when validation fails, got %d", env.Plaid.exchangeCalls)
	}
}

func TestCreateConnection_NotFound_User(t *testing.T) {
	env := setupGenericCreateEnv(t, "full_access")

	body := map[string]any{
		"provider": "plaid",
		"user_id":  "abc12345",
		"credentials": map[string]any{
			"public_token":     "public-sandbox-fake",
			"institution_id":   "ins_109511",
			"institution_name": "Chase",
		},
	}
	resp := env.doPostJSON(t, "/api/v1/connections", body)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
	if env.Plaid.exchangeCalls != 0 {
		t.Errorf("provider should not be called for missing user, got %d", env.Plaid.exchangeCalls)
	}
}

func TestCreateConnection_ProviderError(t *testing.T) {
	env := setupGenericCreateEnv(t, "full_access")
	env.Plaid.exchangeErr = errors.New("plaid: INVALID_PUBLIC_TOKEN")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	body := map[string]any{
		"provider": "plaid",
		"user_id":  pgconv.FormatUUID(user.ID),
		"credentials": map[string]any{
			"public_token":     "public-sandbox-fake",
			"institution_id":   "ins_109511",
			"institution_name": "Chase",
		},
	}
	resp := env.doPostJSON(t, "/api/v1/connections", body)
	readErrorCode(t, resp, http.StatusBadGateway, "PROVIDER_ERROR")
}

func TestCreateConnection_RequiresWriteScope(t *testing.T) {
	env := setupGenericCreateEnv(t, "read_only")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	body := map[string]any{
		"provider": "plaid",
		"user_id":  pgconv.FormatUUID(user.ID),
		"credentials": map[string]any{
			"public_token":     "public-sandbox-fake",
			"institution_id":   "ins_109511",
			"institution_name": "Chase",
		},
	}
	resp := env.doPostJSON(t, "/api/v1/connections", body)
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

// ---- Backward compat: legacy routes still work ----

func TestCreateConnection_BackwardCompat_PlaidExchangeStillWorks(t *testing.T) {
	env := setupGenericCreateEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	body := map[string]any{
		"public_token":     "public-sandbox-fake-public-token",
		"user_id":          pgconv.FormatUUID(user.ID),
		"institution_id":   "ins_109511",
		"institution_name": "Chase",
	}
	resp := env.doPostJSON(t, "/api/v1/connections/plaid/exchange", body)
	assertStatus(t, resp, http.StatusCreated)

	var out plaidExchangeResponse
	parseJSON(t, resp, &out)
	if out.ConnectionID == "" {
		t.Fatal("legacy route should still return connection_id")
	}
	if out.Status != "active" {
		t.Errorf("legacy route should still return status=active, got %q", out.Status)
	}
}

func TestCreateConnection_BackwardCompat_TellerStillWorks(t *testing.T) {
	env := setupGenericCreateEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	body := map[string]any{
		"user_id":          pgconv.FormatUUID(user.ID),
		"institution_id":   "ins_chase",
		"institution_name": "Chase",
		"access_token":     "token-sandbox-fake-teller",
		"enrollment_id":    "enr_abc123",
	}
	resp := env.doPostJSON(t, "/api/v1/connections/teller", body)
	assertStatus(t, resp, http.StatusCreated)

	var out tellerSetupResponse
	parseJSON(t, resp, &out)
	if out.ConnectionID == "" {
		t.Fatal("legacy route should still return connection_id")
	}
}

func TestCreateConnection_BackwardCompat_CSVImportStillWorks(t *testing.T) {
	env := setupGenericCreateEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	seedUncategorized(t, env.Queries)

	body := map[string]any{
		"csv_base64":     base64.StdEncoding.EncodeToString([]byte(sampleCSV)),
		"user_id":        pgconv.FormatUUID(user.ID),
		"column_mapping": sampleCSVColumnMapping(),
		"account_name":   "Legacy CSV",
		"date_format":    "2006-01-02",
	}
	resp := env.doPostJSON(t, "/api/v1/connections/csv/import", body)
	assertStatus(t, resp, http.StatusCreated)

	var out csvImportResponse
	parseJSON(t, resp, &out)
	if out.ConnectionID == "" {
		t.Fatal("legacy route should still return connection_id")
	}
	if out.ImportedTransactions != 2 {
		t.Errorf("want 2 imported, got %d", out.ImportedTransactions)
	}
}
