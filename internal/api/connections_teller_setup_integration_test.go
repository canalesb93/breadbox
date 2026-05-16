//go:build integration && !lite

// Integration tests for the Teller setup REST endpoint
// (POST /api/v1/connections/teller).
//
// Mirrors the fake-provider pattern from
// connections_plaid_link_integration_test.go: a stub provider implementing
// just the ExchangeToken entry point is wired into a *app.App, driven
// through the public REST router with API-key auth + write-scope enforced
// just like production.
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

// fakeTellerProvider implements provider.Provider for the setup-flow tests.
// Only ExchangeToken and Name are functional; everything else panics so any
// unexpected call is loud during testing.
type fakeTellerProvider struct {
	exchangeConn      provider.Connection
	exchangeAccounts  []provider.Account
	exchangeErr       error
	exchangeCalls     int
	lastExchangeToken string
}

func (f *fakeTellerProvider) CreateLinkSession(context.Context, string) (provider.LinkSession, error) {
	return provider.LinkSession{Token: "teller-app-fake"}, nil
}

func (f *fakeTellerProvider) ExchangeToken(_ context.Context, publicToken string) (provider.Connection, []provider.Account, error) {
	f.exchangeCalls++
	f.lastExchangeToken = publicToken
	if f.exchangeErr != nil {
		return provider.Connection{}, nil, f.exchangeErr
	}
	return f.exchangeConn, f.exchangeAccounts, nil
}

func (f *fakeTellerProvider) CreateReauthSession(context.Context, provider.Connection) (provider.LinkSession, error) {
	panic("fakeTellerProvider.CreateReauthSession not implemented")
}
func (f *fakeTellerProvider) SyncTransactions(context.Context, provider.Connection, string) (provider.SyncResult, error) {
	panic("fakeTellerProvider.SyncTransactions not implemented")
}
func (f *fakeTellerProvider) GetBalances(context.Context, provider.Connection) ([]provider.AccountBalance, error) {
	panic("fakeTellerProvider.GetBalances not implemented")
}
func (f *fakeTellerProvider) HandleWebhook(context.Context, provider.WebhookPayload) (provider.WebhookEvent, error) {
	panic("fakeTellerProvider.HandleWebhook not implemented")
}
func (f *fakeTellerProvider) RemoveConnection(context.Context, provider.Connection) error {
	panic("fakeTellerProvider.RemoveConnection not implemented")
}

// tellerSetupEnv is the per-test harness for the setup endpoint.
type tellerSetupEnv struct {
	Server   *httptest.Server
	APIKey   string
	App      *app.App
	Service  *service.Service
	Queries  *db.Queries
	Provider *fakeTellerProvider
}

func setupTellerSetupEnv(t *testing.T, scope string, registerProvider bool) *tellerSetupEnv {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	engine := bsync.NewEngine(queries, pool, nil, slog.Default())
	svc := service.New(queries, pool, engine, slog.Default())

	keyResult, err := svc.CreateAPIKeyLegacy(t.Context(), "teller-setup-test-key", scope)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	fp := &fakeTellerProvider{
		exchangeConn: provider.Connection{
			ProviderName:         "teller",
			ExternalID:           "enr_xyz_999",
			EncryptedCredentials: []byte("encrypted-teller-token"),
			InstitutionName:      "Chase",
		},
		exchangeAccounts: []provider.Account{
			{
				ExternalID:      "acc_chk_provider_1",
				Name:            "Chase Checking",
				Type:            "depository",
				Subtype:         "checking",
				Mask:            "1234",
				ISOCurrencyCode: "USD",
			},
			{
				ExternalID:      "acc_sav_provider_2",
				Name:            "Chase Savings",
				Type:            "depository",
				Subtype:         "savings",
				Mask:            "5678",
				ISOCurrencyCode: "USD",
			},
		},
	}

	providers := map[string]provider.Provider{}
	if registerProvider {
		providers["teller"] = fp
	}
	a := &app.App{
		DB:        pool,
		Queries:   queries,
		Logger:    slog.Default(),
		Service:   svc,
		Providers: providers,
	}

	r := buildTellerSetupTestRouter(svc, a)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	return &tellerSetupEnv{
		Server:   server,
		APIKey:   keyResult.PlaintextKey,
		App:      a,
		Service:  svc,
		Queries:  queries,
		Provider: fp,
	}
}

// buildTellerSetupTestRouter mounts only the route exercised by these tests.
// Mirrors the production wiring: API key auth runs outside the write-scope
// group; the Teller setup endpoint sits inside it.
func buildTellerSetupTestRouter(svc *service.Service, a *app.App) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Post("/connections/teller", TellerSetupHandler(a))
		})
	})
	return r
}

func (e *tellerSetupEnv) doPostJSON(t *testing.T, path string, body any) *http.Response {
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

func validTellerSetupBody(userID string) map[string]any {
	return map[string]any{
		"user_id":          userID,
		"institution_id":   "ins_chase",
		"institution_name": "Chase",
		"access_token":     "token-sandbox-fake-teller",
		"enrollment_id":    "enr_abc123",
		"accounts": []map[string]string{
			{"id": "acc_chk_request_1", "name": "Chase Checking", "type": "depository", "subtype": "checking", "last_four": "1234"},
		},
	}
}

func TestTellerSetup_Success(t *testing.T) {
	env := setupTellerSetupEnv(t, "full_access", true)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/teller",
		validTellerSetupBody(pgconv.FormatUUID(user.ID)))
	assertStatus(t, resp, http.StatusCreated)

	var body tellerSetupResponse
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

	// The provider must receive the JSON-encoded blob with access_token,
	// enrollment_id, and institution_name — that's the contract Teller's
	// provider.ExchangeToken parses (see internal/provider/teller/link.go).
	var seen tellerExchangeBlob
	if err := json.Unmarshal([]byte(env.Provider.lastExchangeToken), &seen); err != nil {
		t.Fatalf("provider received non-JSON publicToken: %v (raw=%q)", err, env.Provider.lastExchangeToken)
	}
	if seen.AccessToken != "token-sandbox-fake-teller" {
		t.Errorf("want forwarded access_token %q, got %q", "token-sandbox-fake-teller", seen.AccessToken)
	}
	if seen.EnrollmentID != "enr_abc123" {
		t.Errorf("want forwarded enrollment_id %q, got %q", "enr_abc123", seen.EnrollmentID)
	}
	if seen.InstitutionName != "Chase" {
		t.Errorf("want forwarded institution_name %q, got %q", "Chase", seen.InstitutionName)
	}

	// Verify DB persistence: connection by short_id with provider's external_id.
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
	if conn.Provider != db.ProviderTypeTeller {
		t.Errorf("want provider=teller, got %q", conn.Provider)
	}
	if conn.ExternalID.String != "enr_xyz_999" {
		t.Errorf("want external_id %q (from provider), got %q", "enr_xyz_999", conn.ExternalID.String)
	}
	if conn.InstitutionName.String != "Chase" {
		t.Errorf("want institution_name %q, got %q", "Chase", conn.InstitutionName.String)
	}
	if string(conn.EncryptedCredentials) != "encrypted-teller-token" {
		t.Errorf("want encrypted credentials persisted verbatim from provider, got %q",
			string(conn.EncryptedCredentials))
	}
}

func TestTellerSetup_BadInput_MissingAccessToken(t *testing.T) {
	env := setupTellerSetupEnv(t, "full_access", true)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	body := validTellerSetupBody(pgconv.FormatUUID(user.ID))
	delete(body, "access_token")

	resp := env.doPostJSON(t, "/api/v1/connections/teller", body)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
	if env.Provider.exchangeCalls != 0 {
		t.Errorf("provider should not be called when validation fails, got %d", env.Provider.exchangeCalls)
	}
}

func TestTellerSetup_BadInput_MissingEnrollmentID(t *testing.T) {
	env := setupTellerSetupEnv(t, "full_access", true)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	body := validTellerSetupBody(pgconv.FormatUUID(user.ID))
	delete(body, "enrollment_id")

	resp := env.doPostJSON(t, "/api/v1/connections/teller", body)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
	if env.Provider.exchangeCalls != 0 {
		t.Errorf("provider should not be called when validation fails, got %d", env.Provider.exchangeCalls)
	}
}

func TestTellerSetup_BadInput_MissingUserID(t *testing.T) {
	env := setupTellerSetupEnv(t, "full_access", true)

	body := validTellerSetupBody("")
	delete(body, "user_id")

	resp := env.doPostJSON(t, "/api/v1/connections/teller", body)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestTellerSetup_BadInput_MissingInstitutionName(t *testing.T) {
	env := setupTellerSetupEnv(t, "full_access", true)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	body := validTellerSetupBody(pgconv.FormatUUID(user.ID))
	delete(body, "institution_name")

	resp := env.doPostJSON(t, "/api/v1/connections/teller", body)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestTellerSetup_NoProvider(t *testing.T) {
	env := setupTellerSetupEnv(t, "full_access", false)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/teller",
		validTellerSetupBody(pgconv.FormatUUID(user.ID)))
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestTellerSetup_ProviderError(t *testing.T) {
	env := setupTellerSetupEnv(t, "full_access", true)
	env.Provider.exchangeErr = errors.New("teller: HTTP 401")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/teller",
		validTellerSetupBody(pgconv.FormatUUID(user.ID)))
	readErrorCode(t, resp, http.StatusBadGateway, "PROVIDER_ERROR")
}

func TestTellerSetup_NotFound_User(t *testing.T) {
	env := setupTellerSetupEnv(t, "full_access", true)

	body := validTellerSetupBody("abc12345") // valid short_id format, no row
	resp := env.doPostJSON(t, "/api/v1/connections/teller", body)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
	if env.Provider.exchangeCalls != 0 {
		t.Errorf("provider should not be called for missing user, got %d", env.Provider.exchangeCalls)
	}
}

func TestTellerSetup_RequiresWriteScope(t *testing.T) {
	env := setupTellerSetupEnv(t, "read_only", true)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/teller",
		validTellerSetupBody(pgconv.FormatUUID(user.ID)))
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

func TestTellerSetup_AcceptsShortIDForUser(t *testing.T) {
	env := setupTellerSetupEnv(t, "full_access", true)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/connections/teller", validTellerSetupBody(user.ShortID))
	assertStatus(t, resp, http.StatusCreated)

	var body tellerSetupResponse
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

func TestTellerSetup_PersistsAccountsFromProvider(t *testing.T) {
	env := setupTellerSetupEnv(t, "full_access", true)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	// Request body has 1 account; provider returns 2. The 2 from the
	// provider are what should land in the DB — matching the Plaid handler's
	// behavior (the request `accounts` field is informational only).
	resp := env.doPostJSON(t, "/api/v1/connections/teller",
		validTellerSetupBody(pgconv.FormatUUID(user.ID)))
	assertStatus(t, resp, http.StatusCreated)

	var body tellerSetupResponse
	parseJSON(t, resp, &body)
	uid, err := env.Service.ResolveConnectionUUID(t.Context(), body.ConnectionID)
	if err != nil {
		t.Fatalf("resolve persisted connection: %v", err)
	}
	accts, err := env.Queries.ListAccountsByConnection(t.Context(), uid)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accts) != 2 {
		t.Fatalf("want 2 accounts persisted from provider response, got %d", len(accts))
	}
	// Sanity: external IDs should match the provider's, not the request's.
	gotExternalIDs := map[string]bool{}
	for _, a := range accts {
		gotExternalIDs[a.ExternalAccountID] = true
	}
	for _, want := range []string{"acc_chk_provider_1", "acc_sav_provider_2"} {
		if !gotExternalIDs[want] {
			t.Errorf("want persisted account external_id %q, missing from %v", want, gotExternalIDs)
		}
	}
	if gotExternalIDs["acc_chk_request_1"] {
		t.Errorf("request-body account external_id leaked into persistence")
	}
}
