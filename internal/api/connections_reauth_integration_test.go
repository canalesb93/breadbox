//go:build integration

// Integration tests for the reauth REST endpoints.
//
// These exercise the provider-touching code path that the rest of the API
// suite skirts, so they wire a fake provider into a *app.App rather than
// re-using the svc-only buildTestRouter helper from api_integration_test.go.
package api

import (
	"context"
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
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeProvider implements provider.Provider for reauth tests. Only the
// methods exercised by the reauth handlers are functional; the rest panic so
// any unexpected call is loud.
type fakeProvider struct {
	name             string
	reauthSession    provider.LinkSession
	reauthErr        error
	reauthCalls      int
	lastReauthConnID string
}

func (f *fakeProvider) CreateReauthSession(_ context.Context, conn provider.Connection) (provider.LinkSession, error) {
	f.reauthCalls++
	f.lastReauthConnID = conn.ExternalID
	if f.reauthErr != nil {
		return provider.LinkSession{}, f.reauthErr
	}
	return f.reauthSession, nil
}

func (f *fakeProvider) CreateLinkSession(context.Context, string) (provider.LinkSession, error) {
	panic("fakeProvider.CreateLinkSession not implemented")
}
func (f *fakeProvider) ExchangeToken(context.Context, string) (provider.Connection, []provider.Account, error) {
	panic("fakeProvider.ExchangeToken not implemented")
}
func (f *fakeProvider) SyncTransactions(context.Context, provider.Connection, string) (provider.SyncResult, error) {
	panic("fakeProvider.SyncTransactions not implemented")
}
func (f *fakeProvider) GetBalances(context.Context, provider.Connection) ([]provider.AccountBalance, error) {
	panic("fakeProvider.GetBalances not implemented")
}
func (f *fakeProvider) HandleWebhook(context.Context, provider.WebhookPayload) (provider.WebhookEvent, error) {
	panic("fakeProvider.HandleWebhook not implemented")
}
func (f *fakeProvider) RemoveConnection(context.Context, provider.Connection) error {
	panic("fakeProvider.RemoveConnection not implemented")
}

// reauthEnv is the per-test harness for the reauth endpoints.
type reauthEnv struct {
	Server   *httptest.Server
	APIKey   string
	App      *app.App
	Service  *service.Service
	Queries  *db.Queries
	Provider *fakeProvider
}

func setupReauthEnv(t *testing.T, scope string) *reauthEnv {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	engine := bsync.NewEngine(queries, pool, nil, slog.Default())
	svc := service.New(queries, pool, engine, slog.Default())

	keyResult, err := svc.CreateAPIKeyLegacy(t.Context(), "reauth-test-key", scope)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	fp := &fakeProvider{
		name: "plaid",
		reauthSession: provider.LinkSession{
			Token:  "link-sandbox-fake-token",
			Expiry: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
		},
	}
	a := &app.App{
		DB:        pool,
		Queries:   queries,
		Logger:    slog.Default(),
		Service:   svc,
		Providers: map[string]provider.Provider{"plaid": fp},
	}

	r := buildReauthTestRouter(svc, a)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	return &reauthEnv{
		Server:   server,
		APIKey:   keyResult.PlaintextKey,
		App:      a,
		Service:  svc,
		Queries:  queries,
		Provider: fp,
	}
}

// buildReauthTestRouter mounts only the routes exercised by these tests
// (auth + reauth pair). Mirrors the production wiring: API key auth runs
// outside the write-scope group; both reauth endpoints sit inside it.
func buildReauthTestRouter(svc *service.Service, a *app.App) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Post("/connections/{id}/reauth", ConnectionReauthHandler(a))
			r.Post("/connections/{id}/reauth-complete", ConnectionReauthCompleteHandler(a))
		})
	})
	return r
}

func (e *reauthEnv) doPost(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("POST", e.Server.URL+path, nil)
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

func TestConnectionReauth_Success(t *testing.T) {
	env := setupReauthEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_reauth_ok")

	resp := env.doPost(t, "/api/v1/connections/"+pgconv.FormatUUID(conn.ID)+"/reauth")
	assertStatus(t, resp, http.StatusOK)

	var body reauthLinkTokenResponse
	parseJSON(t, resp, &body)
	if body.LinkToken != "link-sandbox-fake-token" {
		t.Errorf("want link_token %q, got %q", "link-sandbox-fake-token", body.LinkToken)
	}
	if body.Expiration != "2030-01-02T03:04:05Z" {
		t.Errorf("want expiration %q, got %q", "2030-01-02T03:04:05Z", body.Expiration)
	}
	if env.Provider.reauthCalls != 1 {
		t.Errorf("want 1 provider call, got %d", env.Provider.reauthCalls)
	}
	if env.Provider.lastReauthConnID != "ext_reauth_ok" {
		t.Errorf("want provider received external_id %q, got %q",
			"ext_reauth_ok", env.Provider.lastReauthConnID)
	}
}

func TestConnectionReauth_AcceptsShortID(t *testing.T) {
	env := setupReauthEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_reauth_short")

	resp := env.doPost(t, "/api/v1/connections/"+conn.ShortID+"/reauth")
	assertStatus(t, resp, http.StatusOK)

	var body reauthLinkTokenResponse
	parseJSON(t, resp, &body)
	if body.LinkToken == "" {
		t.Error("want non-empty link_token from short_id lookup")
	}
}

func TestConnectionReauth_NotFound(t *testing.T) {
	env := setupReauthEnv(t, "full_access")
	// Random valid UUID — no matching row.
	resp := env.doPost(t, "/api/v1/connections/00000000-0000-0000-0000-000000000000/reauth")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
	if env.Provider.reauthCalls != 0 {
		t.Errorf("provider should not be called for missing connection, got %d calls", env.Provider.reauthCalls)
	}
}

func TestConnectionReauth_NoProvider(t *testing.T) {
	env := setupReauthEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Bob")
	// Teller connection but only "plaid" registered → INVALID_PARAMETER.
	conn := testutil.MustCreateTellerConnection(t, env.Queries, user.ID, "ext_no_prov")

	resp := env.doPost(t, "/api/v1/connections/"+conn.ShortID+"/reauth")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestConnectionReauth_ProviderError(t *testing.T) {
	env := setupReauthEnv(t, "full_access")
	env.Provider.reauthErr = errors.New("plaid: ITEM_LOGIN_REQUIRED")

	user := testutil.MustCreateUser(t, env.Queries, "Carol")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_reauth_err")

	resp := env.doPost(t, "/api/v1/connections/"+conn.ShortID+"/reauth")
	readErrorCode(t, resp, http.StatusBadGateway, "PROVIDER_ERROR")
}

func TestConnectionReauthComplete_Success(t *testing.T) {
	env := setupReauthEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Dan")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_complete_ok")

	// Push the connection into the broken state we expect to recover from.
	if err := env.Queries.UpdateBankConnectionStatus(t.Context(), db.UpdateBankConnectionStatusParams{
		ID:           conn.ID,
		Status:       db.ConnectionStatusPendingReauth,
		ErrorCode:    pgtype.Text{String: "ITEM_LOGIN_REQUIRED", Valid: true},
		ErrorMessage: pgtype.Text{String: "user revoked access", Valid: true},
	}); err != nil {
		t.Fatalf("seed pending_reauth: %v", err)
	}

	resp := env.doPost(t, "/api/v1/connections/"+conn.ShortID+"/reauth-complete")
	assertStatus(t, resp, http.StatusOK)

	var body map[string]string
	parseJSON(t, resp, &body)
	if body["status"] != "active" {
		t.Errorf("want status=active in body, got %q", body["status"])
	}

	// Confirm DB state: status flipped, error fields cleared.
	got, err := env.Queries.GetBankConnection(t.Context(), conn.ID)
	if err != nil {
		t.Fatalf("reload connection: %v", err)
	}
	if got.Status != db.ConnectionStatusActive {
		t.Errorf("want status=active, got %q", got.Status)
	}
	if got.ErrorCode.Valid {
		t.Errorf("want error_code cleared, got %q", got.ErrorCode.String)
	}
	if got.ErrorMessage.Valid {
		t.Errorf("want error_message cleared, got %q", got.ErrorMessage.String)
	}
}

func TestConnectionReauthComplete_NotFound(t *testing.T) {
	env := setupReauthEnv(t, "full_access")
	resp := env.doPost(t, "/api/v1/connections/00000000-0000-0000-0000-000000000000/reauth-complete")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestConnectionReauth_RequiresWriteScope(t *testing.T) {
	env := setupReauthEnv(t, "read_only")
	user := testutil.MustCreateUser(t, env.Queries, "Eve")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_reauth_scope")

	resp := env.doPost(t, "/api/v1/connections/"+conn.ShortID+"/reauth")
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

func TestConnectionReauthComplete_RequiresWriteScope(t *testing.T) {
	env := setupReauthEnv(t, "read_only")
	user := testutil.MustCreateUser(t, env.Queries, "Frank")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_complete_scope")

	resp := env.doPost(t, "/api/v1/connections/"+conn.ShortID+"/reauth-complete")
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}
