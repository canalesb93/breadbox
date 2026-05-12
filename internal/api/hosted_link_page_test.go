//go:build integration

// Integration tests for the page-internal hosted-link surface:
//
//   GET  /link/{token}                          → standalone HTML
//   GET  /_link/{token}/session                 → redacted session view
//   POST /_link/{token}/providers/{name}/start  → page-scoped link-token start
//   POST /_link/{token}/connections             → page-scoped connection create
//   POST /_link/{token}/complete                → user-initiated "I'm done"
//   POST /_link/{token}/fail                    → SDK-side failure report
//
// We focus on the cross-cutting middleware behavior (bearer rejection,
// expiry, consumption) plus the scope-pinning checks each handler layers on
// top. The Plaid happy-path round-trip is deliberately NOT tested here —
// the existing /api/v1/connections + fakePlaidProvider tests in this
// package already cover the underlying providerEntry.exchange logic, and
// the hosted-link wrapper just delegates to it.
package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

// hostedLinkPageEnv mirrors the genericCreateEnv shape but mounts the
// page-internal /_link/{token}/* routes plus the standalone /link/{token}
// page so we can exercise the full surface from an httptest.Server.
type hostedLinkPageEnv struct {
	Server  *httptest.Server
	App     *app.App
	Service *service.Service
	Queries *db.Queries
}

func setupHostedLinkPageEnv(t *testing.T) *hostedLinkPageEnv {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	engine := bsync.NewEngine(queries, pool, nil, slog.Default())
	svc := service.New(queries, pool, engine, slog.Default())

	fp := &fakePlaidProvider{
		linkSession: provider.LinkSession{
			Token:  "link-sandbox-test",
			Expiry: time.Now().Add(30 * time.Minute),
		},
		exchangeConn: provider.Connection{
			ProviderName:         "plaid",
			ExternalID:           "ext_hosted_link_item",
			EncryptedCredentials: []byte("encrypted-token"),
		},
		exchangeAccounts: []provider.Account{
			{ExternalID: "ext_acct_1", Name: "Plaid Checking", Type: "depository"},
		},
		reauthSession: provider.LinkSession{
			Token:  "link-reauth-test",
			Expiry: time.Now().Add(30 * time.Minute),
		},
	}
	ft := &fakeTellerProvider{
		exchangeConn: provider.Connection{
			ProviderName: "teller", ExternalID: "enr_x", EncryptedCredentials: []byte("e"),
		},
		exchangeAccounts: []provider.Account{
			{ExternalID: "acc_1", Name: "Chase Checking", Type: "depository"},
		},
	}

	a := &app.App{
		DB:        pool,
		Queries:   queries,
		Logger:    slog.Default(),
		Service:   svc,
		Providers: map[string]provider.Provider{"plaid": fp, "teller": ft},
	}

	r := chi.NewRouter()
	r.Get("/link/{token}", HostedLinkPageHandler())
	r.Route("/_link/{token}", func(r chi.Router) {
		r.Use(mw.HostedLinkBearer(svc))
		r.Get("/session", GetHostedLinkPageSessionHandler(svc))
		r.Post("/providers/{name}/start", HostedLinkPageStartHandler(a))
		r.Post("/connections", HostedLinkPageConnectionHandler(a))
		r.Post("/reauth-complete", HostedLinkPageReauthCompleteHandler(svc))
		r.Post("/complete", HostedLinkPageCompleteHandler(svc))
		r.Post("/fail", HostedLinkPageFailHandler(svc))
	})

	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	return &hostedLinkPageEnv{
		Server:  server,
		App:     a,
		Service: svc,
		Queries: queries,
	}
}

// mintSession creates a hosted-link session and returns the session + the
// plaintext token (only available at creation time).
func (e *hostedLinkPageEnv) mintSession(t *testing.T, params service.CreateHostedLinkParams) (service.HostedLinkSession, string) {
	t.Helper()
	if params.UserID == "" {
		u := testutil.MustCreateUser(t, e.Queries, "Alice")
		params.UserID = pgconv.FormatUUID(u.ID)
	}
	if params.Action == "" {
		params.Action = service.HostedLinkActionLink
	}
	res, err := e.Service.CreateHostedLink(context.Background(), params)
	if err != nil {
		t.Fatalf("mint hosted-link session: %v", err)
	}
	return res.Session, res.Token
}

// doReq is a tiny helper because these endpoints don't carry an API key —
// the token in the path is the credential, so we don't need testEnv.
func (e *hostedLinkPageEnv) doReq(t *testing.T, method, path string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, e.Server.URL+path, body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func TestHostedLink_Bearer_RejectsUnknownToken(t *testing.T) {
	env := setupHostedLinkPageEnv(t)
	resp := env.doReq(t, "GET", "/_link/does-not-exist/session", nil)
	readErrorCode(t, resp, http.StatusUnauthorized, "INVALID_TOKEN")
}

func TestHostedLink_Bearer_RejectsExpired(t *testing.T) {
	env := setupHostedLinkPageEnv(t)
	sess, token := env.mintSession(t, service.CreateHostedLinkParams{Provider: "plaid"})

	// Push expires_at into the past directly via SQL — the service has no
	// "set expiry" knob and the cleanup job would set status=expired, which
	// the middleware also catches. We want to assert the past-expiry guard
	// fires even when the underlying status is still pending.
	_, err := env.App.DB.Exec(context.Background(),
		`UPDATE hosted_link_sessions SET expires_at = NOW() - INTERVAL '1 minute' WHERE id = $1`,
		mustParseUUID(t, sess.ID),
	)
	if err != nil {
		t.Fatalf("update expires_at: %v", err)
	}

	resp := env.doReq(t, "GET", "/_link/"+token+"/session", nil)
	readErrorCode(t, resp, http.StatusGone, "EXPIRED")
}

func TestHostedLink_Bearer_RejectsCompleted(t *testing.T) {
	env := setupHostedLinkPageEnv(t)
	sess, token := env.mintSession(t, service.CreateHostedLinkParams{Provider: "plaid"})
	// Drive it through pending → active → completed via service methods.
	if err := env.Service.MarkHostedLinkStarted(context.Background(), sess.ID); err != nil {
		t.Fatalf("mark started: %v", err)
	}
	if err := env.Service.CompleteHostedLink(context.Background(), sess.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}

	resp := env.doReq(t, "GET", "/_link/"+token+"/session", nil)
	readErrorCode(t, resp, http.StatusGone, "CONSUMED")
}

func TestHostedLink_Session_FlipsPendingToActive(t *testing.T) {
	env := setupHostedLinkPageEnv(t)
	_, token := env.mintSession(t, service.CreateHostedLinkParams{Provider: "plaid", Label: "Chase"})

	// First call: flips pending → active and returns status=active.
	resp := env.doReq(t, "GET", "/_link/"+token+"/session", nil)
	assertStatus(t, resp, http.StatusOK)
	var first struct {
		Status string `json:"status"`
		Label  string `json:"label"`
	}
	parseJSON(t, resp, &first)
	if first.Status != "active" {
		t.Fatalf("first session call status: want active, got %q", first.Status)
	}
	if first.Label != "Chase" {
		t.Errorf("expected label to round-trip, got %q", first.Label)
	}

	// Second call: idempotent — still active, no error.
	resp = env.doReq(t, "GET", "/_link/"+token+"/session", nil)
	assertStatus(t, resp, http.StatusOK)
	var second struct {
		Status string `json:"status"`
	}
	parseJSON(t, resp, &second)
	if second.Status != "active" {
		t.Fatalf("second session call status: want active, got %q", second.Status)
	}
}

func TestHostedLink_Session_OmitsAttribution(t *testing.T) {
	env := setupHostedLinkPageEnv(t)
	_, token := env.mintSession(t, service.CreateHostedLinkParams{Provider: "plaid"})

	resp := env.doReq(t, "GET", "/_link/"+token+"/session", nil)
	assertStatus(t, resp, http.StatusOK)
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	body := string(data)
	if strings.Contains(body, `"user_id"`) {
		t.Errorf("session response leaks user_id: %s", body)
	}
	// Look for the scalar "connection_id" key specifically — the array key
	// "result_connection_ids" is allowed (and required) on this surface.
	if strings.Contains(body, `"connection_id"`) {
		t.Errorf("session response leaks connection_id: %s", body)
	}
}

func TestHostedLink_StartScopedToProvider(t *testing.T) {
	env := setupHostedLinkPageEnv(t)
	// Pin the session to plaid; calling /providers/teller/start must 403.
	_, token := env.mintSession(t, service.CreateHostedLinkParams{Provider: "plaid"})

	resp := env.doReq(t, "POST", "/_link/"+token+"/providers/teller/start", strings.NewReader("{}"))
	readErrorCode(t, resp, http.StatusForbidden, "FORBIDDEN")

	// The pinned provider works (returns a link_token from fakePlaidProvider).
	resp = env.doReq(t, "POST", "/_link/"+token+"/providers/plaid/start", strings.NewReader("{}"))
	assertStatus(t, resp, http.StatusOK)
	var out struct {
		LinkToken string `json:"link_token"`
	}
	parseJSON(t, resp, &out)
	if out.LinkToken == "" {
		t.Fatal("expected link_token from fake plaid provider")
	}
}

func TestHostedLink_Page_RendersHTML(t *testing.T) {
	env := setupHostedLinkPageEnv(t)
	_, token := env.mintSession(t, service.CreateHostedLinkParams{Provider: "plaid"})

	resp := env.doReq(t, "GET", "/link/"+token, nil)
	assertStatus(t, resp, http.StatusOK)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("want text/html content-type, got %q", ct)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	got := string(body)
	if !strings.Contains(got, "hosted-link") {
		t.Errorf("hosted-link page missing data-hosted-link marker: %s", got[:min(len(got), 200)])
	}
	// The token must NOT be rendered into the HTML — the JS reads it from
	// window.location. Catching this regression prevents leaking the
	// credential into HTML caches / shared screenshots.
	if strings.Contains(got, token) {
		t.Errorf("hosted-link page rendered the token into HTML — must come from window.location only")
	}
}

func TestHostedLink_Complete_Idempotent(t *testing.T) {
	env := setupHostedLinkPageEnv(t)
	sess, token := env.mintSession(t, service.CreateHostedLinkParams{Provider: "plaid"})
	// Move pending → active so the service accepts a complete call.
	if err := env.Service.MarkHostedLinkStarted(context.Background(), sess.ID); err != nil {
		t.Fatalf("mark started: %v", err)
	}

	resp := env.doReq(t, "POST", "/_link/"+token+"/complete", strings.NewReader("{}"))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("first complete: want 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	updated, err := env.Service.GetHostedLinkSession(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updated.Status != "completed" {
		t.Errorf("status after complete: want completed, got %q", updated.Status)
	}

	// Second complete: the bearer middleware now rejects with 410 CONSUMED
	// because the session is terminal. This is the intended idempotency —
	// the page's "I'm done" button can be spammed without 5xx.
	resp = env.doReq(t, "POST", "/_link/"+token+"/complete", strings.NewReader("{}"))
	readErrorCode(t, resp, http.StatusGone, "CONSUMED")
}

func TestHostedLink_Fail_TerminatesSession(t *testing.T) {
	env := setupHostedLinkPageEnv(t)
	sess, token := env.mintSession(t, service.CreateHostedLinkParams{Provider: "plaid"})

	resp := env.doReq(t, "POST", "/_link/"+token+"/fail",
		strings.NewReader(`{"code":"USER_CANCEL","message":"User cancelled in Plaid"}`))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("fail: want 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	updated, err := env.Service.GetHostedLinkSession(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updated.Status != "failed" {
		t.Errorf("status after fail: want failed, got %q", updated.Status)
	}
	if updated.ErrorCode != "USER_CANCEL" {
		t.Errorf("error_code: want USER_CANCEL, got %q", updated.ErrorCode)
	}
}

// mustParseUUID is a small helper for tests that need to talk SQL directly.
func mustParseUUID(t *testing.T, s string) any {
	t.Helper()
	u, err := pgconv.ParseUUID(s)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return u
}

