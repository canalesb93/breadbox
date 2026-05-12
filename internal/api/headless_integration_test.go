//go:build integration

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"breadbox/internal/app"
	"breadbox/internal/config"
	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
	bsync "breadbox/internal/sync"
	"breadbox/internal/testutil"

	"github.com/go-chi/chi/v5"
)

// TestHeadlessBootstrap exercises GET /api/v1/headless/bootstrap end-to-end:
// the handler stitches process-scoped facts (encryption key, providers,
// scheduler) onto DB-derived counts. We don't run a scheduler in the test
// (it would race with other integration tests), so SchedulerRunning stays
// false; everything else is covered.
func TestHeadlessBootstrap(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	engine := bsync.NewEngine(queries, pool, nil, slog.Default())
	svc := service.New(queries, pool, engine, slog.Default())

	keyResult, err := svc.CreateAPIKey(t.Context(), "headless-test", "read_only")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	cfg := &config.Config{
		EncryptionKey: []byte("01234567890123456789012345678901"), // 32 bytes
		PlaidClientID: "plaid-test-id",
		PlaidEnv:      "sandbox",
		// Teller deliberately unconfigured.
	}
	a := &app.App{
		DB:      pool,
		Queries: queries,
		Config:  cfg,
		Logger:  slog.Default(),
	}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))
		r.Get("/headless/bootstrap", HeadlessBootstrapHandler(svc, a, "v0.test"))
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	req, err := http.NewRequest("GET", srv.URL+"/api/v1/headless/bootstrap", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-API-Key", keyResult.PlaintextKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}

	var report service.HeadlessBootstrapResponse
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if report.Version != "v0.test" {
		t.Errorf("version: got %q, want v0.test", report.Version)
	}
	if !report.EncryptionKeySet {
		t.Errorf("encryption_key_set: got false, want true")
	}
	if !report.Database.Connected {
		t.Errorf("database.connected: got false, want true")
	}
	if report.Database.MigrationsCurrent != true {
		t.Errorf("database.migrations_current: got false (applied=%d) — test DB should have all migrations applied via TestMain", report.Database.MigrationVersion)
	}
	// First-run: fresh test DB has no api key created by service — but we
	// created one above, and no login accounts. So first_run should be true
	// (login_accounts_count is the gate, not api_keys_count).
	if !report.FirstRun {
		t.Errorf("first_run: got false, want true (no login accounts in fixture)")
	}
	if report.LoginAccountsCount != 0 {
		t.Errorf("login_accounts_count: got %d, want 0", report.LoginAccountsCount)
	}
	if report.APIKeysCount < 1 {
		t.Errorf("api_keys_count: got %d, want >=1 (we created one)", report.APIKeysCount)
	}

	// Provider rows — order is stable.
	if len(report.Providers) != 2 {
		t.Fatalf("providers: got %d rows, want 2", len(report.Providers))
	}
	if report.Providers[0].Name != "plaid" || !report.Providers[0].Configured || report.Providers[0].Env != "sandbox" {
		t.Errorf("providers[0] plaid: %+v", report.Providers[0])
	}
	if report.Providers[1].Name != "teller" || report.Providers[1].Configured {
		t.Errorf("providers[1] teller: %+v (want unconfigured)", report.Providers[1])
	}

	// Scheduler is not running in this test fixture.
	if report.SchedulerRunning {
		t.Errorf("scheduler_running: got true, want false (no scheduler in fixture)")
	}
}

// TestHeadlessBootstrap_NoEncryptionKey verifies the encryption_key_set
// field flips when the key is unset.
func TestHeadlessBootstrap_NoEncryptionKey(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	engine := bsync.NewEngine(queries, pool, nil, slog.Default())
	svc := service.New(queries, pool, engine, slog.Default())

	keyResult, err := svc.CreateAPIKey(t.Context(), "headless-no-key", "read_only")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	a := &app.App{
		DB:      pool,
		Queries: queries,
		Config:  &config.Config{}, // EncryptionKey nil
		Logger:  slog.Default(),
	}
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))
		r.Get("/headless/bootstrap", HeadlessBootstrapHandler(svc, a, "v0.test"))
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/headless/bootstrap", nil)
	req.Header.Set("X-API-Key", keyResult.PlaintextKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	var report service.HeadlessBootstrapResponse
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if report.EncryptionKeySet {
		t.Errorf("encryption_key_set: got true, want false")
	}
}
