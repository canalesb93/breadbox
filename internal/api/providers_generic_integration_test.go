//go:build integration

// Integration tests for the generic provider registry endpoints
// (GET /api/v1/providers, GET /api/v1/providers/{name},
//  POST /api/v1/providers/{name}/link-session).
//
// Uses the same fake-provider pattern as
// connections_plaid_link_integration_test.go and
// connections_teller_setup_integration_test.go — the fake providers from
// those test files are reused here so we don't duplicate stubs.
package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"breadbox/internal/app"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	"breadbox/internal/provider"
	"breadbox/internal/service"
	bsync "breadbox/internal/sync"
	"breadbox/internal/testutil"

	"github.com/go-chi/chi/v5"
)

// providersEnv is the harness for the registry tests. It bundles an app
// with both Plaid and Teller fakes registered so list/get can observe
// `configured: true` for them.
type providersEnv struct {
	Server   *httptest.Server
	APIKey   string
	App      *app.App
	Service  *service.Service
	Plaid    *fakePlaidProvider
	Teller   *fakeTellerProvider
}

func setupProvidersEnv(t *testing.T, scope string, registerPlaid, registerTeller bool) *providersEnv {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	engine := bsync.NewEngine(queries, pool, nil, slog.Default())
	svc := service.New(queries, pool, engine, slog.Default())

	keyResult, err := svc.CreateAPIKeyLegacy(t.Context(), "providers-test-key", scope)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	fp := &fakePlaidProvider{
		linkSession: provider.LinkSession{
			Token:  "link-sandbox-fake-token",
			Expiry: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
		},
	}
	ft := &fakeTellerProvider{}

	providers := map[string]provider.Provider{}
	if registerPlaid {
		providers["plaid"] = fp
	}
	if registerTeller {
		providers["teller"] = ft
	}
	a := &app.App{
		DB:        pool,
		Queries:   queries,
		Logger:    slog.Default(),
		Service:   svc,
		Providers: providers,
	}

	r := buildProvidersTestRouter(svc, a)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	return &providersEnv{
		Server:  server,
		APIKey:  keyResult.PlaintextKey,
		App:     a,
		Service: svc,
		Plaid:   fp,
		Teller:  ft,
	}
}

// buildProvidersTestRouter mirrors the production wiring for the four
// generic provider endpoints plus the legacy CSV import the CSV exchange
// path reuses.
func buildProvidersTestRouter(svc *service.Service, a *app.App) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))
		// Read endpoints are outside the write-scope group.
		r.Get("/providers", ListProvidersHandler(a))
		r.Get("/providers/{name}", GetProviderHandler(a))
		// Write endpoints — full_access only.
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Post("/providers/{name}/link-session", LinkSessionHandler(a))
			r.Post("/connections", CreateConnectionHandler(a))
		})
	})
	return r
}

func (e *providersEnv) doGet(t *testing.T, path string) *http.Response {
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

func (e *providersEnv) doPostJSON(t *testing.T, path string, body any) *http.Response {
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

// ---- List / Get ----

func TestListProviders_ReturnsAll(t *testing.T) {
	env := setupProvidersEnv(t, "full_access", true, true)

	resp := env.doGet(t, "/api/v1/providers")
	assertStatus(t, resp, http.StatusOK)

	var out []providerInfo
	parseJSON(t, resp, &out)
	if len(out) < 3 {
		t.Fatalf("want at least 3 providers, got %d", len(out))
	}
	gotByName := map[string]providerInfo{}
	for _, p := range out {
		gotByName[p.Name] = p
	}
	for _, want := range []string{"plaid", "teller", "csv"} {
		if _, ok := gotByName[want]; !ok {
			t.Errorf("missing provider %q in response", want)
		}
	}
	if !gotByName["plaid"].Configured {
		t.Errorf("plaid should be configured (provider registered), got configured=false")
	}
	if !gotByName["teller"].Configured {
		t.Errorf("teller should be configured (provider registered), got configured=false")
	}
	if !gotByName["csv"].Configured {
		t.Errorf("csv should always be configured (no external deps), got configured=false")
	}
	if !gotByName["plaid"].NeedsLinkSession {
		t.Errorf("plaid should need link session")
	}
	if gotByName["teller"].NeedsLinkSession {
		t.Errorf("teller should not need link session")
	}
	if gotByName["csv"].NeedsLinkSession {
		t.Errorf("csv should not need link session")
	}
}

func TestListProviders_UnconfiguredStillAppears(t *testing.T) {
	env := setupProvidersEnv(t, "full_access", false, false)

	resp := env.doGet(t, "/api/v1/providers")
	assertStatus(t, resp, http.StatusOK)

	var out []providerInfo
	parseJSON(t, resp, &out)
	gotByName := map[string]providerInfo{}
	for _, p := range out {
		gotByName[p.Name] = p
	}
	if gotByName["plaid"].Configured {
		t.Errorf("plaid should NOT be configured (provider absent)")
	}
	if gotByName["teller"].Configured {
		t.Errorf("teller should NOT be configured (provider absent)")
	}
}

func TestListProviders_SchemaIncludesRequiredFields(t *testing.T) {
	env := setupProvidersEnv(t, "full_access", true, true)

	resp := env.doGet(t, "/api/v1/providers")
	assertStatus(t, resp, http.StatusOK)

	var out []providerInfo
	parseJSON(t, resp, &out)
	gotByName := map[string]providerInfo{}
	for _, p := range out {
		gotByName[p.Name] = p
	}
	// Spot-check that each provider's schema names the canonical required
	// fields. The schema map values aren't structurally enforced by the
	// type system; this is the regression guard.
	plaid := gotByName["plaid"].CredentialsSchema
	for _, key := range []string{"public_token", "institution_id", "institution_name"} {
		if _, ok := plaid[key]; !ok {
			t.Errorf("plaid schema missing field %q", key)
		}
	}
	teller := gotByName["teller"].CredentialsSchema
	for _, key := range []string{"access_token", "enrollment_id", "institution_name"} {
		if _, ok := teller[key]; !ok {
			t.Errorf("teller schema missing field %q", key)
		}
	}
	csv := gotByName["csv"].CredentialsSchema
	for _, key := range []string{"csv_base64", "column_mapping"} {
		if _, ok := csv[key]; !ok {
			t.Errorf("csv schema missing field %q", key)
		}
	}
}

func TestGetProvider_Plaid(t *testing.T) {
	env := setupProvidersEnv(t, "full_access", true, true)

	resp := env.doGet(t, "/api/v1/providers/plaid")
	assertStatus(t, resp, http.StatusOK)

	var out providerInfo
	parseJSON(t, resp, &out)
	if out.Name != "plaid" {
		t.Errorf("want name=plaid, got %q", out.Name)
	}
	if !out.Configured {
		t.Errorf("want configured=true (provider registered)")
	}
	if !out.NeedsLinkSession {
		t.Errorf("want needs_link_session=true for plaid")
	}
}

func TestGetProvider_Unknown(t *testing.T) {
	env := setupProvidersEnv(t, "full_access", true, true)

	resp := env.doGet(t, "/api/v1/providers/moneykit")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// ---- Link session ----

func TestLinkSession_Plaid(t *testing.T) {
	env := setupProvidersEnv(t, "full_access", true, true)
	user := testutil.MustCreateUser(t, env.App.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/providers/plaid/link-session", map[string]string{
		"user_id": pgconv.FormatUUID(user.ID),
	})
	assertStatus(t, resp, http.StatusOK)

	var body linkSessionResponse
	parseJSON(t, resp, &body)
	if body.LinkToken != "link-sandbox-fake-token" {
		t.Errorf("want link_token %q, got %q", "link-sandbox-fake-token", body.LinkToken)
	}
	if body.Expiration != "2030-01-02T03:04:05Z" {
		t.Errorf("want expiration %q, got %q", "2030-01-02T03:04:05Z", body.Expiration)
	}
	if env.Plaid.linkCalls != 1 {
		t.Errorf("want 1 link call to provider, got %d", env.Plaid.linkCalls)
	}
}

func TestLinkSession_Teller(t *testing.T) {
	env := setupProvidersEnv(t, "full_access", true, true)
	user := testutil.MustCreateUser(t, env.App.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/providers/teller/link-session", map[string]string{
		"user_id": pgconv.FormatUUID(user.ID),
	})
	assertStatus(t, resp, http.StatusNoContent)
}

func TestLinkSession_CSV(t *testing.T) {
	env := setupProvidersEnv(t, "full_access", true, true)
	user := testutil.MustCreateUser(t, env.App.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/providers/csv/link-session", map[string]string{
		"user_id": pgconv.FormatUUID(user.ID),
	})
	assertStatus(t, resp, http.StatusNoContent)
}

func TestLinkSession_UnknownProvider(t *testing.T) {
	env := setupProvidersEnv(t, "full_access", true, true)
	user := testutil.MustCreateUser(t, env.App.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/providers/moneykit/link-session", map[string]string{
		"user_id": pgconv.FormatUUID(user.ID),
	})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestLinkSession_RequiresWriteScope(t *testing.T) {
	env := setupProvidersEnv(t, "read_only", true, true)
	user := testutil.MustCreateUser(t, env.App.Queries, "Alice")

	resp := env.doPostJSON(t, "/api/v1/providers/plaid/link-session", map[string]string{
		"user_id": pgconv.FormatUUID(user.ID),
	})
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}
