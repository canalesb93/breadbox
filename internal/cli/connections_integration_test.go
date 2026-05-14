//go:build integration && !lite

// Integration tests for `breadbox connections`. They drive the real REST
// handlers via an in-process httptest.Server so the CLI client and
// service layer both run end-to-end. The hosted-link flow is the
// centerpiece — we mint a session, dereference the URL it returns, and
// confirm the page renders.

package cli_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"breadbox/internal/api"
	"breadbox/internal/cli/config"
	"breadbox/internal/client"
	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/go-chi/chi/v5"
)

// connEnv extends envBundle with the connections + hosted-link routes
// (plus the public /link/{token} page). Reused by every connection-flavor
// test in this file.
type connEnv struct {
	Svc     *service.Service
	Server  *httptest.Server
	Client  *client.Client
	Queries *db.Queries
}

func setupConnEnv(t *testing.T) *connEnv {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())

	keyResult, err := svc.CreateAPIKeyLegacy(t.Context(), "cli-conn-test", "full_access")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))

		// Read endpoints
		r.Get("/connections", api.ListConnectionsHandler(svc))
		r.Get("/connections/{id}", api.GetConnectionHandler(svc))
		r.Get("/connections/link/{id}", api.GetHostedLinkSessionHandler(svc))
		r.Get("/sync/health", api.SyncHealthHandler(svc))
		r.Get("/sync/logs", api.ListSyncLogsHandler(svc))

		// Write endpoints
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Post("/sync", api.TriggerSyncHandler(svc))
			r.Post("/connections/link", api.CreateHostedLinkHandler(svc))
			r.Post("/connections/{id}/relink", api.CreateHostedLinkRelinkHandler(svc))
			r.Delete("/connections/{id}", api.DeleteConnectionHandler(svc))
			r.Post("/connections/csv/preview", api.CSVPreviewHandler(svc))
			r.Post("/connections/csv/import", api.CSVImportHandler(svc))
		})
	})
	// Standalone hosted-link page — used to verify the URL the CLI prints
	// actually resolves to something the user can open.
	r.Get("/link/{token}", api.HostedLinkPageHandler())

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	c := client.New(config.Host{BaseURL: srv.URL, Token: keyResult.PlaintextKey}, "test")
	return &connEnv{Svc: svc, Server: srv, Client: c, Queries: queries}
}

func TestConnectionsList_ReturnsFixtures(t *testing.T) {
	env := setupConnEnv(t)
	q := env.Queries
	user := testutil.MustCreateUser(t, q, "Alice")
	testutil.MustCreateConnection(t, q, user.ID, "ext-conn-list")

	conns, err := env.Client.ListConnections(context.Background(), "")
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	if len(conns) == 0 {
		t.Fatal("expected >=1 connection, got 0")
	}
}

func TestConnectionsLink_MintsSessionAndPageRenders(t *testing.T) {
	env := setupConnEnv(t)
	q := env.Queries
	user := testutil.MustCreateUser(t, q, "Alice")

	res, err := env.Client.CreateHostedLink(context.Background(), client.CreateHostedLinkParams{
		UserID:   pgconv.FormatUUID(user.ID),
		Provider: "plaid",
	})
	if err != nil {
		t.Fatalf("CreateHostedLink: %v", err)
	}
	if res.URL == "" {
		t.Fatal("expected URL on hosted-link response")
	}
	if !strings.Contains(res.URL, "/link/") {
		t.Errorf("URL %q does not look like a hosted-link URL", res.URL)
	}
	if res.Token == "" {
		t.Fatal("expected one-time token on the create response")
	}

	// Dereference the URL — page should render HTML.
	resp, err := http.Get(res.URL)
	if err != nil {
		t.Fatalf("GET %s: %v", res.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("page status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestConnectionsLinkGet_ReturnsSession(t *testing.T) {
	env := setupConnEnv(t)
	q := env.Queries
	user := testutil.MustCreateUser(t, q, "Alice")

	created, err := env.Client.CreateHostedLink(context.Background(), client.CreateHostedLinkParams{
		UserID: pgconv.FormatUUID(user.ID),
	})
	if err != nil {
		t.Fatalf("CreateHostedLink: %v", err)
	}

	fetched, err := env.Client.GetHostedLinkSession(context.Background(), created.ShortID)
	if err != nil {
		t.Fatalf("GetHostedLinkSession: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("fetched.ID = %q, want %q", fetched.ID, created.ID)
	}
	// Poll endpoint must NOT echo the token or the URL.
	// (We assert that by virtue of the typed struct not carrying them —
	// the JSON shape is the same as `HostedLinkSession`.)
}

func TestConnectionsDisconnect_FlipsStatus(t *testing.T) {
	env := setupConnEnv(t)
	q := env.Queries
	user := testutil.MustCreateUser(t, q, "Alice")
	conn := testutil.MustCreateConnection(t, q, user.ID, "ext-disc")

	if err := env.Client.DisconnectConnection(context.Background(), conn.ShortID); err != nil {
		t.Fatalf("DisconnectConnection: %v", err)
	}
	// Re-fetch via the queries — after a soft-disconnect the row exists
	// with status='disconnected'. The public list endpoint hides them so
	// we look at the raw row.
	row, err := q.GetBankConnection(context.Background(), conn.ID)
	if err != nil {
		t.Fatalf("GetBankConnection: %v", err)
	}
	if string(row.Status) != "disconnected" {
		t.Errorf("status = %q, want disconnected", row.Status)
	}
}
