//go:build integration

// Integration tests for the Plaid link-flow REST endpoints
// (POST /api/v1/connections/plaid/link-token,
//  POST /api/v1/connections/plaid/exchange).
//
// Mirrors the fake-provider pattern from
// connections_reauth_integration_test.go: a stub provider implementing the
// link-session + exchange-token entry points is wired into a *app.App,
// driven through the public REST router with API-key auth + write-scope
// enforced just like production.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// fakePlaidProvider implements provider.Provider for the link-flow tests.
// CreateLinkSession, ExchangeToken, and CreateReauthSession are functional
// (CreateReauthSession is needed by the hosted-link relink page tests).
// Everything else panics so any unexpected call is loud during testing.
type fakePlaidProvider struct {
	linkSession        provider.LinkSession
	linkErr            error
	linkCalls          int
	lastLinkUserID     string
	exchangeConn      provider.Connection
	exchangeAccounts  []provider.Account
	exchangeErr       error
	exchangeCalls     int
	lastExchangeToken string
	reauthSession    provider.LinkSession
	reauthErr        error
	reauthCalls      int
	lastReauthExtID  string
}

func (f *fakePlaidProvider) CreateLinkSession(_ context.Context, userID string) (provider.LinkSession, error) {
	f.linkCalls++
	f.lastLinkUserID = userID
	if f.linkErr != nil {
		return provider.LinkSession{}, f.linkErr
	}
	return f.linkSession, nil
}

func (f *fakePlaidProvider) ExchangeToken(_ context.Context, publicToken string) (provider.Connection, []provider.Account, error) {
	f.exchangeCalls++
	f.lastExchangeToken = publicToken
	if f.exchangeErr != nil {
		return provider.Connection{}, nil, f.exchangeErr
	}
	return f.exchangeConn, f.exchangeAccounts, nil
}

func (f *fakePlaidProvider) CreateReauthSession(_ context.Context, conn provider.Connection) (provider.LinkSession, error) {
	f.reauthCalls++
	f.lastReauthExtID = conn.ExternalID
	if f.reauthErr != nil {
		return provider.LinkSession{}, f.reauthErr
	}
	return f.reauthSession, nil
}
func (f *fakePlaidProvider) SyncTransactions(context.Context, provider.Connection, string) (provider.SyncResult, error) {
	panic("fakePlaidProvider.SyncTransactions not implemented")
}
func (f *fakePlaidProvider) GetBalances(context.Context, provider.Connection) ([]provider.AccountBalance, error) {
	panic("fakePlaidProvider.GetBalances not implemented")
}
func (f *fakePlaidProvider) HandleWebhook(context.Context, provider.WebhookPayload) (provider.WebhookEvent, error) {
	panic("fakePlaidProvider.HandleWebhook not implemented")
}
func (f *fakePlaidProvider) RemoveConnection(context.Context, provider.Connection) error {
	panic("fakePlaidProvider.RemoveConnection not implemented")
}

// plaidLinkEnv is the per-test harness for the link-flow endpoints.
type plaidLinkEnv struct {
	Server   *httptest.Server
	APIKey   string
	App      *app.App
	Service  *service.Service
	Queries  *db.Queries
	Provider *fakePlaidProvider
}

func setupPlaidLinkEnv(t *testing.T, scope string, registerProvider bool) *plaidLinkEnv {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	engine := bsync.NewEngine(queries, pool, nil, slog.Default())
	svc := service.New(queries, pool, engine, slog.Default())

	keyResult, err := svc.CreateAPIKeyLegacy(t.Context(), "plaid-link-test-key", scope)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	fp := &fakePlaidProvider{
		linkSession: provider.LinkSession{
			Token:  "link-sandbox-fake-token",
			Expiry: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
		},
		exchangeConn: provider.Connection{
			ProviderName:         "plaid",
			ExternalID:           "ext_new_item_123",
			EncryptedCredentials: []byte("encrypted-access-token"),
		},
		exchangeAccounts: []provider.Account{
			{
				ExternalID:      "ext_acct_chk_1",
				Name:            "Plaid Checking",
				OfficialName:    "Plaid Gold Standard 0% Interest Checking",
				Type:            "depository",
				Subtype:         "checking",
				Mask:            "0000",
				ISOCurrencyCode: "USD",
			},
			{
				ExternalID:      "ext_acct_sav_1",
				Name:            "Plaid Saving",
				Type:            "depository",
				Subtype:         "savings",
				Mask:            "1111",
				ISOCurrencyCode: "USD",
			},
		},
	}

	providers := map[string]provider.Provider{}
	if registerProvider {
		providers["plaid"] = fp
	}
	a := &app.App{
		DB:        pool,
		Queries:   queries,
		Logger:    slog.Default(),
		Service:   svc,
		Providers: providers,
	}

	r := buildPlaidLinkTestRouter(svc, a)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	return &plaidLinkEnv{
		Server:   server,
		APIKey:   keyResult.PlaintextKey,
		App:      a,
		Service:  svc,
		Queries:  queries,
		Provider: fp,
	}
}

// buildPlaidLinkTestRouter mounts only the routes exercised by these tests.
// Mirrors the production wiring: API key auth runs outside the write-scope
// group; both Plaid endpoints sit inside it.
func buildPlaidLinkTestRouter(svc *service.Service, a *app.App) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Post("/connections/plaid/link-token", PlaidLinkTokenHandler(a))
			r.Post("/connections/plaid/exchange", PlaidExchangeHandler(a))
		})
	})
	return r
}

func (e *plaidLinkEnv) doPostJSON(t *testing.T, path string, body any) *http.Response {
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

// ---- Link-token tests ----

func TestPlaidLinkToken_Success(t *testing.T) {
	env := setupPlaidLinkEnv(t, "full_access", true)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/plaid/link-token", map[string]string{
		"user_id": pgconv.FormatUUID(user.ID),
	})
	assertStatus(t, resp, http.StatusOK)

	var body plaidLinkTokenResponse
	parseJSON(t, resp, &body)
	if body.LinkToken != "link-sandbox-fake-token" {
		t.Errorf("want link_token %q, got %q", "link-sandbox-fake-token", body.LinkToken)
	}
	if body.Expiration != "2030-01-02T03:04:05Z" {
		t.Errorf("want expiration %q, got %q", "2030-01-02T03:04:05Z", body.Expiration)
	}
	if env.Provider.linkCalls != 1 {
		t.Errorf("want 1 provider call, got %d", env.Provider.linkCalls)
	}
	if env.Provider.lastLinkUserID != pgconv.FormatUUID(user.ID) {
		t.Errorf("want provider received user_id %q, got %q",
			pgconv.FormatUUID(user.ID), env.Provider.lastLinkUserID)
	}
}

func TestPlaidLinkToken_AcceptsShortIDForUser(t *testing.T) {
	env := setupPlaidLinkEnv(t, "full_access", true)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/plaid/link-token", map[string]string{
		"user_id": user.ShortID,
	})
	assertStatus(t, resp, http.StatusOK)

	// Provider should always see the canonical UUID (not the short_id) so
	// retries land on the same Plaid client_user_id regardless of caller
	// preference at the API edge.
	if env.Provider.lastLinkUserID != pgconv.FormatUUID(user.ID) {
		t.Errorf("want canonical UUID forwarded to provider, got %q", env.Provider.lastLinkUserID)
	}
}

func TestPlaidLinkToken_NoProvider(t *testing.T) {
	env := setupPlaidLinkEnv(t, "full_access", false)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/plaid/link-token", map[string]string{
		"user_id": pgconv.FormatUUID(user.ID),
	})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestPlaidLinkToken_ProviderError(t *testing.T) {
	env := setupPlaidLinkEnv(t, "full_access", true)
	env.Provider.linkErr = errors.New("plaid: INVALID_REQUEST")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/plaid/link-token", map[string]string{
		"user_id": pgconv.FormatUUID(user.ID),
	})
	readErrorCode(t, resp, http.StatusBadGateway, "PROVIDER_ERROR")
}

func TestPlaidLinkToken_NotFound_User(t *testing.T) {
	env := setupPlaidLinkEnv(t, "full_access", true)

	resp := env.doPostJSON(t, "/api/v1/connections/plaid/link-token", map[string]string{
		"user_id": "abc12345", // valid short_id format but no matching row
	})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
	if env.Provider.linkCalls != 0 {
		t.Errorf("provider should not be called for missing user, got %d calls", env.Provider.linkCalls)
	}
}

func TestPlaidLinkToken_BadInput_MissingUserID(t *testing.T) {
	env := setupPlaidLinkEnv(t, "full_access", true)

	resp := env.doPostJSON(t, "/api/v1/connections/plaid/link-token", map[string]string{})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestPlaidLinkToken_RequiresWriteScope(t *testing.T) {
	env := setupPlaidLinkEnv(t, "read_only", true)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/plaid/link-token", map[string]string{
		"user_id": pgconv.FormatUUID(user.ID),
	})
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

// ---- Exchange tests ----

func validExchangeBody(userID string) map[string]any {
	return map[string]any{
		"public_token":     "public-sandbox-fake-public-token",
		"user_id":          userID,
		"institution_id":   "ins_109511",
		"institution_name": "Chase",
		"accounts": []map[string]string{
			{"id": "ext_acct_chk_1", "name": "Plaid Checking", "type": "depository", "subtype": "checking", "mask": "0000"},
		},
	}
}

func TestPlaidExchange_Success(t *testing.T) {
	env := setupPlaidLinkEnv(t, "full_access", true)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/plaid/exchange",
		validExchangeBody(pgconv.FormatUUID(user.ID)))
	assertStatus(t, resp, http.StatusCreated)

	var body plaidExchangeResponse
	parseJSON(t, resp, &body)
	if body.ConnectionID == "" {
		t.Fatal("want non-empty connection_id (short_id) in response")
	}
	if body.InstitutionName != "Chase" {
		t.Errorf("want institution_name %q, got %q", "Chase", body.InstitutionName)
	}
	if body.Status != "active" {
		t.Errorf("want status %q, got %q", "active", body.Status)
	}
	if env.Provider.exchangeCalls != 1 {
		t.Errorf("want 1 provider exchange call, got %d", env.Provider.exchangeCalls)
	}
	if env.Provider.lastExchangeToken != "public-sandbox-fake-public-token" {
		t.Errorf("want provider received public_token %q, got %q",
			"public-sandbox-fake-public-token", env.Provider.lastExchangeToken)
	}

	// Verify DB persistence: connection by short_id, with both accounts
	// from the provider response (NOT the request body).
	uid, err := env.Service.ResolveConnectionUUID(t.Context(), body.ConnectionID)
	if err != nil {
		t.Fatalf("resolve persisted connection short_id: %v", err)
	}
	conn, err := env.Queries.GetBankConnection(t.Context(), uid)
	if err != nil {
		t.Fatalf("reload connection: %v", err)
	}
	if conn.Status != db.ConnectionStatusActive {
		t.Errorf("want connection status=active, got %q", conn.Status)
	}
	if conn.Provider != db.ProviderTypePlaid {
		t.Errorf("want provider=plaid, got %q", conn.Provider)
	}
	if conn.ExternalID.String != "ext_new_item_123" {
		t.Errorf("want external_id %q, got %q", "ext_new_item_123", conn.ExternalID.String)
	}
	if conn.InstitutionName.String != "Chase" {
		t.Errorf("want institution_name %q, got %q", "Chase", conn.InstitutionName.String)
	}
	if string(conn.EncryptedCredentials) != "encrypted-access-token" {
		t.Errorf("want encrypted credentials persisted verbatim from provider, got %q",
			string(conn.EncryptedCredentials))
	}

	accts, err := env.Queries.ListAccountsByConnection(t.Context(), uid)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accts) != 2 {
		t.Fatalf("want 2 accounts persisted from provider response, got %d", len(accts))
	}
}

func TestPlaidExchange_AcceptsShortIDForUser(t *testing.T) {
	env := setupPlaidLinkEnv(t, "full_access", true)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/plaid/exchange", validExchangeBody(user.ShortID))
	assertStatus(t, resp, http.StatusCreated)

	var body plaidExchangeResponse
	parseJSON(t, resp, &body)
	if body.ConnectionID == "" {
		t.Fatal("want non-empty connection_id from short_id-based call")
	}

	// The persisted connection should belong to the resolved user.
	uid, _ := env.Service.ResolveConnectionUUID(t.Context(), body.ConnectionID)
	conn, err := env.Queries.GetBankConnection(t.Context(), uid)
	if err != nil {
		t.Fatalf("reload connection: %v", err)
	}
	if pgconv.FormatUUID(conn.UserID) != pgconv.FormatUUID(user.ID) {
		t.Errorf("want connection.user_id %q, got %q",
			pgconv.FormatUUID(user.ID), pgconv.FormatUUID(conn.UserID))
	}
}

func TestPlaidExchange_BadInput_MissingPublicToken(t *testing.T) {
	env := setupPlaidLinkEnv(t, "full_access", true)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	body := validExchangeBody(pgconv.FormatUUID(user.ID))
	delete(body, "public_token")

	resp := env.doPostJSON(t, "/api/v1/connections/plaid/exchange", body)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
	if env.Provider.exchangeCalls != 0 {
		t.Errorf("provider should not be called when validation fails, got %d", env.Provider.exchangeCalls)
	}
}

func TestPlaidExchange_NoProvider(t *testing.T) {
	env := setupPlaidLinkEnv(t, "full_access", false)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/plaid/exchange",
		validExchangeBody(pgconv.FormatUUID(user.ID)))
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestPlaidExchange_ProviderError(t *testing.T) {
	env := setupPlaidLinkEnv(t, "full_access", true)
	env.Provider.exchangeErr = errors.New("plaid: INVALID_PUBLIC_TOKEN")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/plaid/exchange",
		validExchangeBody(pgconv.FormatUUID(user.ID)))
	readErrorCode(t, resp, http.StatusBadGateway, "PROVIDER_ERROR")
}

func TestPlaidExchange_NotFound_User(t *testing.T) {
	env := setupPlaidLinkEnv(t, "full_access", true)

	body := validExchangeBody("abc12345") // valid short_id format, no row
	resp := env.doPostJSON(t, "/api/v1/connections/plaid/exchange", body)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
	if env.Provider.exchangeCalls != 0 {
		t.Errorf("provider should not be called for missing user, got %d", env.Provider.exchangeCalls)
	}
}

func TestPlaidExchange_RequiresWriteScope(t *testing.T) {
	env := setupPlaidLinkEnv(t, "read_only", true)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/plaid/exchange",
		validExchangeBody(pgconv.FormatUUID(user.ID)))
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}
