//go:build integration

// Integration tests for the relink REST endpoint:
//
//	POST /api/v1/connections/{id}/relink
//
// Plus an end-to-end coverage test for the bearer-side reauth-complete
// path that the standalone page hits on a successful Plaid update-mode
// flow.
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
	"github.com/jackc/pgx/v5/pgtype"
)

// relinkEnv mirrors the per-test harness shape used by the link-flow tests,
// but mounts the full hosted-link surface — the agent REST endpoint plus
// the page-internal reauth-complete handler — against a single httptest
// server so the round-trip is realistic.
type relinkEnv struct {
	Server   *httptest.Server
	APIKey   string
	App      *app.App
	Service  *service.Service
	Queries  *db.Queries
	Provider *fakePlaidProvider
}

func setupRelinkEnv(t *testing.T, scope string) *relinkEnv {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	engine := bsync.NewEngine(queries, pool, nil, slog.Default())
	svc := service.New(queries, pool, engine, slog.Default())

	keyResult, err := svc.CreateAPIKeyLegacy(t.Context(), "relink-test-key", scope)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	fp := &fakePlaidProvider{
		linkSession: provider.LinkSession{
			Token:  "link-sandbox-test",
			Expiry: time.Now().Add(30 * time.Minute),
		},
		reauthSession: provider.LinkSession{
			Token:  "link-reauth-fake",
			Expiry: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
		},
		exchangeConn: provider.Connection{
			ProviderName: "plaid", ExternalID: "ext_x", EncryptedCredentials: []byte("e"),
		},
		exchangeAccounts: []provider.Account{{ExternalID: "a", Name: "n", Type: "depository"}},
	}
	a := &app.App{
		DB:        pool,
		Queries:   queries,
		Logger:    slog.Default(),
		Service:   svc,
		Providers: map[string]provider.Provider{"plaid": fp},
	}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Post("/connections/{id}/relink", CreateHostedLinkRelinkHandler(svc))
		})
	})
	r.Route("/_link/{token}", func(r chi.Router) {
		r.Use(mw.HostedLinkBearer(svc))
		r.Get("/session", GetHostedLinkPageSessionHandler(svc))
		r.Post("/providers/{name}/start", HostedLinkPageStartHandler(a))
		r.Post("/reauth-complete", HostedLinkPageReauthCompleteHandler(svc))
	})

	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	return &relinkEnv{
		Server:   server,
		APIKey:   keyResult.PlaintextKey,
		App:      a,
		Service:  svc,
		Queries:  queries,
		Provider: fp,
	}
}

func (e *relinkEnv) doRelink(t *testing.T, id string, body any) *http.Response {
	t.Helper()
	te := &testEnv{Server: e.Server, APIKey: e.APIKey}
	return te.doPost(t, "/api/v1/connections/"+id+"/relink", body)
}

// doBearer is a tokenized request helper — the page-internal surface uses
// the token in the URL as its credential, no API key.
func (e *relinkEnv) doBearer(t *testing.T, method, token, path string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, e.Server.URL+"/_link/"+token+path, body)
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

func TestRelink_HappyPath(t *testing.T) {
	env := setupRelinkEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_relink_ok")

	resp := env.doRelink(t, conn.ShortID, map[string]any{
		"label":              "Chase checking re-auth",
		"expires_in_seconds": 900,
	})
	assertStatus(t, resp, http.StatusCreated)

	var body hostedLinkResponseBody
	parseJSON(t, resp, &body)
	if body.Token == "" {
		t.Fatal("expected non-empty token on relink create response")
	}
	if body.Action != "relink" {
		t.Errorf("expected action=relink, got %q", body.Action)
	}
	if !body.SingleUse {
		t.Error("expected single_use=true for relink sessions")
	}
	if body.Provider != "plaid" {
		t.Errorf("expected provider=plaid derived from connection, got %q", body.Provider)
	}
	if body.Status != "pending" {
		t.Errorf("expected status=pending, got %q", body.Status)
	}
	if body.Label != "Chase checking re-auth" {
		t.Errorf("expected label to round-trip, got %q", body.Label)
	}
	if !strings.Contains(body.URL, "/link/"+body.Token) {
		t.Errorf("expected url to embed token, got %q", body.URL)
	}

	// Reload via the GET endpoint to confirm connection_id is recorded.
	got, err := env.Service.GetHostedLinkSession(context.Background(), body.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if got.ConnectionID != pgconv.FormatUUID(conn.ID) {
		t.Errorf("expected connection_id %s on session, got %q", pgconv.FormatUUID(conn.ID), got.ConnectionID)
	}
}

func TestRelink_RequiresWriteScope(t *testing.T) {
	env := setupRelinkEnv(t, "read_only")
	user := testutil.MustCreateUser(t, env.Queries, "Bob")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_relink_scope")

	resp := env.doRelink(t, conn.ShortID, map[string]any{})
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

func TestRelink_UnknownConnection(t *testing.T) {
	env := setupRelinkEnv(t, "full_access")
	// Random UUID — no row.
	resp := env.doRelink(t, "00000000-0000-0000-0000-000000000000", map[string]any{})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestRelink_DisconnectedConnection(t *testing.T) {
	env := setupRelinkEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Carol")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_relink_disc")

	// Flip the connection to disconnected — re-auth against this state is
	// nonsensical and the service surfaces ErrInvalidState.
	if err := env.Queries.UpdateBankConnectionStatus(t.Context(), db.UpdateBankConnectionStatusParams{
		ID:           conn.ID,
		Status:       db.ConnectionStatusDisconnected,
		ErrorCode:    pgtype.Text{},
		ErrorMessage: pgtype.Text{},
	}); err != nil {
		t.Fatalf("seed disconnected: %v", err)
	}

	resp := env.doRelink(t, conn.ShortID, map[string]any{})
	readErrorCode(t, resp, http.StatusConflict, "CONNECTION_DISCONNECTED")
}

func TestRelink_InvalidExpiry(t *testing.T) {
	env := setupRelinkEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Dan")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_relink_exp")

	resp := env.doRelink(t, conn.ShortID, map[string]any{"expires_in_seconds": 3601})
	readErrorCode(t, resp, http.StatusBadRequest, "VALIDATION_ERROR")
}

// TestRelink_ReauthCompleteIdempotent walks the page-internal callback
// twice. After /reauth-complete the session is terminal (completed,
// single-use), so the bearer middleware rejects the second call with
// 410 CONSUMED — exactly the contract we want the JS to rely on.
func TestRelink_ReauthCompleteIdempotent(t *testing.T) {
	env := setupRelinkEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Eve")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_relink_idem")

	// Push the connection into pending_reauth so we can observe the
	// active-state transition after /reauth-complete.
	if err := env.Queries.UpdateBankConnectionStatus(t.Context(), db.UpdateBankConnectionStatusParams{
		ID:           conn.ID,
		Status:       db.ConnectionStatusPendingReauth,
		ErrorCode:    pgtype.Text{String: "ITEM_LOGIN_REQUIRED", Valid: true},
		ErrorMessage: pgtype.Text{String: "broken", Valid: true},
	}); err != nil {
		t.Fatalf("seed pending_reauth: %v", err)
	}

	// Mint via the public endpoint to exercise both surfaces together.
	resp := env.doRelink(t, conn.ShortID, map[string]any{})
	assertStatus(t, resp, http.StatusCreated)
	var minted hostedLinkResponseBody
	parseJSON(t, resp, &minted)
	if minted.Token == "" {
		t.Fatal("mint returned empty token")
	}

	// Drive the session to active by hitting /session (the JS does this on
	// page load).
	resp = env.doBearer(t, "GET", minted.Token, "/session", nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// First /reauth-complete: 204, connection flips to active.
	resp = env.doBearer(t, "POST", minted.Token, "/reauth-complete", strings.NewReader("{}"))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("first reauth-complete: want 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	got, err := env.Queries.GetBankConnection(t.Context(), conn.ID)
	if err != nil {
		t.Fatalf("reload connection: %v", err)
	}
	if got.Status != db.ConnectionStatusActive {
		t.Errorf("after reauth-complete: want status=active, got %q", got.Status)
	}
	if got.ErrorCode.Valid {
		t.Errorf("expected error_code cleared, got %q", got.ErrorCode.String)
	}

	sess, err := env.Service.GetHostedLinkSession(context.Background(), minted.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if sess.Status != "completed" {
		t.Errorf("session status after reauth-complete: want completed, got %q", sess.Status)
	}

	// Second /reauth-complete: middleware rejects with 410 CONSUMED.
	resp = env.doBearer(t, "POST", minted.Token, "/reauth-complete", strings.NewReader("{}"))
	readErrorCode(t, resp, http.StatusGone, "CONSUMED")
}

// TestRelink_LinkSessionRejectsReauthComplete: the bearer reauth-complete
// path is gated to action="relink" — a session minted as "link" must 403.
// Belt-and-suspenders coverage; the page never wires this combo, but a
// stray client shouldn't be able to call it either.
func TestRelink_LinkSessionRejectsReauthComplete(t *testing.T) {
	env := setupRelinkEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Frank")

	// Mint a regular link session and drive it to active.
	res, err := env.Service.CreateHostedLink(context.Background(), service.CreateHostedLinkParams{
		UserID:   pgconv.FormatUUID(user.ID),
		Provider: "plaid",
		Action:   service.HostedLinkActionLink,
	})
	if err != nil {
		t.Fatalf("mint link session: %v", err)
	}

	resp := env.doBearer(t, "GET", res.Token, "/session", nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = env.doBearer(t, "POST", res.Token, "/reauth-complete", strings.NewReader("{}"))
	readErrorCode(t, resp, http.StatusForbidden, "FORBIDDEN")
}

// TestRelink_StartReturnsReauthToken asserts the loosened start handler:
// for a relink session it routes through CreateReauthSession on the pinned
// connection, not CreateLinkSession. We assert via the fake provider's
// call counters that the right path fired.
func TestRelink_StartReturnsReauthToken(t *testing.T) {
	env := setupRelinkEnv(t, "full_access")
	user := testutil.MustCreateUser(t, env.Queries, "Greta")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_relink_start")

	resp := env.doRelink(t, conn.ShortID, map[string]any{})
	assertStatus(t, resp, http.StatusCreated)
	var minted hostedLinkResponseBody
	parseJSON(t, resp, &minted)

	resp = env.doBearer(t, "GET", minted.Token, "/session", nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = env.doBearer(t, "POST", minted.Token, "/providers/plaid/start", strings.NewReader("{}"))
	assertStatus(t, resp, http.StatusOK)
	var out struct {
		LinkToken string `json:"link_token"`
	}
	parseJSON(t, resp, &out)
	if out.LinkToken != "link-reauth-fake" {
		t.Errorf("expected reauth-flow link token from fake provider, got %q", out.LinkToken)
	}
	if env.Provider.reauthCalls != 1 {
		t.Errorf("expected 1 CreateReauthSession call, got %d", env.Provider.reauthCalls)
	}
	if env.Provider.linkCalls != 0 {
		t.Errorf("CreateLinkSession should not fire on relink, got %d calls", env.Provider.linkCalls)
	}
	if env.Provider.lastReauthExtID != "ext_relink_start" {
		t.Errorf("provider received wrong external_id: %q", env.Provider.lastReauthExtID)
	}
}
