//go:build integration

// Integration tests for Phase-2 page-polish surface:
//
//   GET  /_link/{token}/providers/teller/config
//
// Plus a redirect_url smoke test asserting POST /_link/{token}/complete still
// returns 204 — the redirect itself happens client-side, the server does not
// emit a Location header.
package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/config"
	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	"breadbox/internal/provider"
	"breadbox/internal/service"
	bsync "breadbox/internal/sync"
	"breadbox/internal/testutil"

	"github.com/go-chi/chi/v5"
)

// hostedLinkTellerEnv mounts only the page-internal routes needed for the
// teller-config + complete tests. We construct a real *config.Config so the
// new handler can read TellerAppID + TellerEnv off App.Config.
type hostedLinkTellerEnv struct {
	Server  *httptest.Server
	App     *app.App
	Service *service.Service
	Queries *db.Queries
}

func setupHostedLinkTellerEnv(t *testing.T, cfg *config.Config) *hostedLinkTellerEnv {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	engine := bsync.NewEngine(queries, pool, nil, slog.Default())
	svc := service.New(queries, pool, engine, slog.Default())

	fp := &fakePlaidProvider{
		linkSession: provider.LinkSession{
			Token:  "link-sandbox-test",
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
		Config:    cfg,
		Logger:    slog.Default(),
		Service:   svc,
		Providers: map[string]provider.Provider{"plaid": fp, "teller": ft},
	}

	r := chi.NewRouter()
	r.Route("/_link/{token}", func(r chi.Router) {
		r.Use(mw.HostedLinkBearer(svc))
		r.Get("/session", GetHostedLinkPageSessionHandler(svc))
		r.Get("/providers/teller/config", HostedLinkPageTellerConfigHandler(a))
		r.Post("/complete", HostedLinkPageCompleteHandler(svc))
	})
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	return &hostedLinkTellerEnv{
		Server:  server,
		App:     a,
		Service: svc,
		Queries: queries,
	}
}

func (e *hostedLinkTellerEnv) mintSession(t *testing.T, p service.CreateHostedLinkParams) (service.HostedLinkSession, string) {
	t.Helper()
	if p.UserID == "" {
		u := testutil.MustCreateUser(t, e.Queries, "TellerTest")
		p.UserID = pgconv.FormatUUID(u.ID)
	}
	if p.Action == "" {
		p.Action = service.HostedLinkActionLink
	}
	res, err := e.Service.CreateHostedLink(context.Background(), p)
	if err != nil {
		t.Fatalf("mint hosted-link session: %v", err)
	}
	return res.Session, res.Token
}

func (e *hostedLinkTellerEnv) doReq(t *testing.T, method, path string, body io.Reader) *http.Response {
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

// configuredTellerCfg returns a *config.Config wired with the public
// Teller bootstrap fields (app id + env). Cert/key PEMs are deliberately
// omitted — the bearer config handler must never read them.
func configuredTellerCfg() *config.Config {
	return &config.Config{
		TellerAppID: "app_teller_test_123",
		TellerEnv:   "sandbox",
	}
}

func TestHostedLink_TellerConfig_HappyPath(t *testing.T) {
	env := setupHostedLinkTellerEnv(t, configuredTellerCfg())
	// Open-picker session — no provider pin, so teller is allowed.
	_, token := env.mintSession(t, service.CreateHostedLinkParams{})

	resp := env.doReq(t, "GET", "/_link/"+token+"/providers/teller/config", nil)
	assertStatus(t, resp, http.StatusOK)
	var got struct {
		ApplicationID string `json:"application_id"`
		Environment   string `json:"environment"`
	}
	parseJSON(t, resp, &got)
	if got.ApplicationID != "app_teller_test_123" {
		t.Errorf("application_id: want app_teller_test_123, got %q", got.ApplicationID)
	}
	if got.Environment != "sandbox" {
		t.Errorf("environment: want sandbox, got %q", got.Environment)
	}
}

func TestHostedLink_TellerConfig_RejectsPlaidPinned(t *testing.T) {
	env := setupHostedLinkTellerEnv(t, configuredTellerCfg())
	// Pin to plaid; the teller config endpoint must 403.
	_, token := env.mintSession(t, service.CreateHostedLinkParams{Provider: "plaid"})

	resp := env.doReq(t, "GET", "/_link/"+token+"/providers/teller/config", nil)
	readErrorCode(t, resp, http.StatusForbidden, "FORBIDDEN")
}

func TestHostedLink_TellerConfig_HidesCertificate(t *testing.T) {
	cfg := configuredTellerCfg()
	// Wire fake PEM bytes — these must NEVER appear in the response. The
	// handler returns a fixed struct that has no cert/key field, but we
	// assert at the byte level too to catch future regressions if someone
	// "helpfully" reflects more of *config.Config back to the page.
	cfg.TellerCertPEM = []byte("-----BEGIN CERTIFICATE-----\nSECRET-CERT\n-----END CERTIFICATE-----")
	cfg.TellerKeyPEM = []byte("-----BEGIN PRIVATE KEY-----\nSECRET-KEY\n-----END PRIVATE KEY-----")
	env := setupHostedLinkTellerEnv(t, cfg)
	_, token := env.mintSession(t, service.CreateHostedLinkParams{Provider: "teller"})

	resp := env.doReq(t, "GET", "/_link/"+token+"/providers/teller/config", nil)
	assertStatus(t, resp, http.StatusOK)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)

	bannedSubstrings := []string{
		"BEGIN CERTIFICATE", "SECRET-CERT",
		"BEGIN PRIVATE KEY", "SECRET-KEY",
		"certificate", "private_key", "cert_pem", "key_pem",
	}
	for _, banned := range bannedSubstrings {
		if strings.Contains(body, banned) {
			t.Errorf("teller config response leaks %q: %s", banned, body)
		}
	}
	// Re-parse to confirm only the two public fields are populated.
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected exactly 2 keys (application_id, environment), got %d: %v", len(got), got)
	}
}

func TestHostedLink_TellerConfig_NotConfigured(t *testing.T) {
	// Empty config — TellerAppID is unset; handler must 400 cleanly rather
	// than surface a half-configured response.
	env := setupHostedLinkTellerEnv(t, &config.Config{})
	_, token := env.mintSession(t, service.CreateHostedLinkParams{Provider: "teller"})

	resp := env.doReq(t, "GET", "/_link/"+token+"/providers/teller/config", nil)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

// TestHostedLink_CompleteRedirectsWhenSet verifies that the bearer-side
// /complete handler is unaffected by redirect_url — it still returns 204,
// and the page is responsible for navigating client-side. We assert this
// because future maintainers might be tempted to issue a 303 here; that
// would break the SPA's "show 'closed' on no-redirect" branch.
func TestHostedLink_CompleteRedirectsWhenSet(t *testing.T) {
	env := setupHostedLinkTellerEnv(t, configuredTellerCfg())
	sess, token := env.mintSession(t, service.CreateHostedLinkParams{
		Provider:    "plaid",
		RedirectURL: "https://chat.example.com/return?ok=1",
	})
	if err := env.Service.MarkHostedLinkStarted(context.Background(), sess.ID); err != nil {
		t.Fatalf("mark started: %v", err)
	}

	resp := env.doReq(t, "POST", "/_link/"+token+"/complete", strings.NewReader("{}"))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("complete with redirect_url: want 204, got %d", resp.StatusCode)
	}
	// No-redirect contract: the server must not emit a Location header.
	if loc := resp.Header.Get("Location"); loc != "" {
		t.Errorf("complete should not emit Location header, got %q", loc)
	}
	resp.Body.Close()
}
