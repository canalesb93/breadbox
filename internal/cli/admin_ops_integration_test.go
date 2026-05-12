//go:build integration

// Integration tests for the providers / config / webhooks CLI commands.
// They drive the real REST handlers through an in-process httptest.Server,
// using the same setup pattern as accounts_integration_test.go.
package cli_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"

	"breadbox/internal/api"
	"breadbox/internal/app"
	"breadbox/internal/cli"
	"breadbox/internal/cli/config"
	"breadbox/internal/client"
	bbconfig "breadbox/internal/config"
	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/go-chi/chi/v5"
)

// adminOpsEnv mirrors envBundle but plugs in the real *app.App that the new
// providers/config/webhooks handlers depend on. We intentionally pass a nil
// SyncEngine — replay tests only need to verify the 200/triggered=false path
// for events without a connection.
type adminOpsEnv struct {
	App     *app.App
	Server  *httptest.Server
	Client  *client.Client
	Queries *db.Queries
	Svc     *service.Service
}

func setupAdminOpsEnv(t *testing.T) *adminOpsEnv {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())

	keyResult, err := svc.CreateAPIKeyLegacy(t.Context(), "cli-test-ops", "full_access")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	cfg := &bbconfig.Config{
		ConfigSources: map[string]string{},
	}
	a := &app.App{
		DB:        pool,
		Queries:   queries,
		Config:    cfg,
		Logger:    slog.Default(),
		Providers: nil,
		Service:   svc,
	}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Get("/config", api.ListConfigHandler(a))
			r.Get("/config/{key}", api.GetConfigHandler(a))
			r.Put("/config/{key}", api.SetConfigHandler(a))
			r.Delete("/config/{key}", api.DeleteConfigHandler(a))
			r.Get("/webhook-events", api.ListWebhookEventsHandler(svc))
			r.Post("/webhook-events/{id}/replay", api.ReplayWebhookEventHandler(svc, nil))
		})
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	c := client.New(config.Host{BaseURL: srv.URL, Token: keyResult.PlaintextKey}, "test")
	return &adminOpsEnv{App: a, Server: srv, Client: c, Queries: queries, Svc: svc}
}

func TestConfig_SetGetUnsetRoundtrip(t *testing.T) {
	env := setupAdminOpsEnv(t)
	ctx := context.Background()

	saved, err := env.Client.SetConfig(ctx, "sync_interval_minutes", "60")
	if err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if saved.Source != "db" {
		t.Errorf("source = %q, want db", saved.Source)
	}

	got, err := env.Client.GetConfig(ctx, "sync_interval_minutes", false)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if got.Value == nil || *got.Value != "60" {
		t.Fatalf("value = %v, want 60", got.Value)
	}
	if got.Source != "db" {
		t.Errorf("source = %q, want db", got.Source)
	}

	if err := env.Client.DeleteConfig(ctx, "sync_interval_minutes"); err != nil {
		t.Fatalf("DeleteConfig: %v", err)
	}

	after, err := env.Client.GetConfig(ctx, "sync_interval_minutes", false)
	if err != nil {
		t.Fatalf("GetConfig after delete: %v", err)
	}
	if after.Value != nil && *after.Value != "" {
		t.Errorf("value after delete = %v, want empty/nil", *after.Value)
	}
}

func TestConfig_ListMasksSecrets(t *testing.T) {
	env := setupAdminOpsEnv(t)
	ctx := context.Background()

	if _, err := env.Client.SetConfig(ctx, "plaid_secret", "supersecretvaluexyz123"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	entries, err := env.Client.ListConfig(ctx, false)
	if err != nil {
		t.Fatalf("ListConfig: %v", err)
	}
	var found *client.ConfigEntry
	for i := range entries {
		if entries[i].Key == "plaid_secret" {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatal("plaid_secret missing from listing")
	}
	if !found.Masked {
		t.Errorf("expected masked=true for plaid_secret, got %+v", found)
	}
	if found.Value != nil && *found.Value == "supersecretvaluexyz123" {
		t.Errorf("plaintext leaked in list: %s", *found.Value)
	}

	// reveal=true should return the raw value.
	revealed, err := env.Client.ListConfig(ctx, true)
	if err != nil {
		t.Fatalf("ListConfig reveal: %v", err)
	}
	var rev *client.ConfigEntry
	for i := range revealed {
		if revealed[i].Key == "plaid_secret" {
			rev = &revealed[i]
			break
		}
	}
	if rev == nil || rev.Value == nil || *rev.Value != "supersecretvaluexyz123" {
		t.Errorf("reveal=true did not return raw value; got %+v", rev)
	}
}

func TestConfig_DeniedKeyRejectsWrite(t *testing.T) {
	env := setupAdminOpsEnv(t)
	ctx := context.Background()
	_, err := env.Client.SetConfig(ctx, "ENCRYPTION_KEY", "not-allowed")
	if err == nil {
		t.Fatal("expected error setting ENCRYPTION_KEY via API")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T %v", err, err)
	}
	if apiErr.Status != 403 {
		t.Errorf("status = %d, want 403", apiErr.Status)
	}
}

func TestWebhooks_TailReturnsEvents(t *testing.T) {
	env := setupAdminOpsEnv(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, env.Queries, "Tester")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext-conn-webhook-1")

	for i := 0; i < 3; i++ {
		_, err := env.Queries.CreateWebhookEvent(ctx, db.CreateWebhookEventParams{
			Provider:       db.ProviderTypePlaid,
			EventType:      "SYNC_UPDATES_AVAILABLE",
			ConnectionID:   conn.ID,
			RawPayloadHash: "hash-" + string(rune('a'+i)),
			Status:         "received",
		})
		if err != nil {
			t.Fatalf("CreateWebhookEvent: %v", err)
		}
	}

	res, err := env.Client.ListWebhookEvents(ctx, client.WebhookEventFilters{Limit: 5})
	if err != nil {
		t.Fatalf("ListWebhookEvents: %v", err)
	}
	if len(res.WebhookEvents) != 3 {
		t.Fatalf("got %d events, want 3", len(res.WebhookEvents))
	}
	if res.WebhookEvents[0].Provider != "plaid" {
		t.Errorf("provider = %q, want plaid", res.WebhookEvents[0].Provider)
	}
}

func TestInit_NonInteractiveCreatesAdminAndKey(t *testing.T) {
	// testutil.ServicePool truncates every entity table before each run; the
	// integration harness ensures we start with no admin accounts, which is
	// exactly the precondition init checks for.
	dir := t.TempDir()
	t.Setenv("BREADBOX_CONFIG_DIR", dir)
	t.Setenv("DATABASE_URL", "postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	// Re-establish a fresh pool — this also resets the truncation set so the
	// auth_accounts row created by previous tests is gone.
	_, _ = testutil.ServicePool(t)

	ctx := context.Background()
	envFile := dir + "/.env"
	if err := cli.RunInitForTest(ctx, envFile, "init-admin@example.com", "topsecret-pw", "Init-Admin"); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	// hosts.toml should now carry a 'local' host with a real token.
	hosts, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	h, name, err := hosts.Get("local")
	if err != nil {
		t.Fatalf("hosts.Get: %v", err)
	}
	if name != "local" {
		t.Errorf("host name = %q, want local", name)
	}
	if h.Token == "" {
		t.Error("hosts.toml has no token after init")
	}
}


func TestWebhooks_ReplayUnknownEventReturns404(t *testing.T) {
	env := setupAdminOpsEnv(t)
	ctx := context.Background()
	_, err := env.Client.ReplayWebhookEvent(ctx, "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected 404 for unknown id")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T %v", err, err)
	}
	if apiErr.Status != 404 {
		t.Errorf("status = %d, want 404", apiErr.Status)
	}
}
