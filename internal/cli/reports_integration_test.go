//go:build integration && !lite

// Integration tests for the reports noun group.
package cli_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"testing"

	"breadbox/internal/api"
	"breadbox/internal/cli/config"
	"breadbox/internal/client"
	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/go-chi/chi/v5"
)

func setupReportsEnv(t *testing.T) *envBundle {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())

	keyResult, err := svc.CreateAPIKeyLegacy(t.Context(), "cli-test", "full_access")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))
		r.Get("/reports", api.ListReportsHandler(svc))
		r.Get("/reports/{id}", api.GetReportHandler(svc))
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Post("/reports", api.CreateReportHandler(svc))
			r.Patch("/reports/{id}/read", api.MarkReportReadHandler(svc))
			r.Patch("/reports/{id}/unread", api.MarkReportUnreadHandler(svc))
		})
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	c := client.New(config.Host{BaseURL: srv.URL, Token: keyResult.PlaintextKey}, "test")
	return &envBundle{Svc: svc, Server: srv, Client: c, Queries: queries}
}

func TestReports_SubmitListRead(t *testing.T) {
	env := setupReportsEnv(t)

	body := []byte(`{"title":"weekly review","body":"all good","priority":"info","tags":["weekly"]}`)
	created, err := env.Client.CreateReport(context.Background(), json.RawMessage(body))
	if err != nil {
		t.Fatalf("CreateReport: %v", err)
	}
	if created.Title != "weekly review" {
		t.Fatalf("title = %q, want weekly review", created.Title)
	}

	list, err := env.Client.ListReports(context.Background())
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	if len(list) == 0 {
		t.Fatalf("expected at least one report, got 0")
	}

	if err := env.Client.MarkReportRead(context.Background(), created.ID); err != nil {
		t.Fatalf("MarkReportRead: %v", err)
	}
	got, err := env.Client.GetReport(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if got.ReadAt == nil {
		t.Fatalf("expected read_at to be populated after MarkReportRead")
	}

	if err := env.Client.MarkReportUnread(context.Background(), created.ID); err != nil {
		t.Fatalf("MarkReportUnread: %v", err)
	}
	got, err = env.Client.GetReport(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetReport after unread: %v", err)
	}
	if got.ReadAt != nil {
		t.Fatalf("expected read_at to be nil after MarkReportUnread, got %v", *got.ReadAt)
	}
}
