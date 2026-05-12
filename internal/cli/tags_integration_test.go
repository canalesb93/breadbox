//go:build integration && !lite

// Integration tests for the tags noun group.
package cli_test

import (
	"context"
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

func setupTagsEnv(t *testing.T) *envBundle {
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
		r.Get("/tags", api.ListTagsHandler(svc))
		r.Get("/tags/{slug}", api.GetTagHandler(svc))
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Post("/tags", api.CreateTagHandler(svc))
			r.Patch("/tags/{slug}", api.UpdateTagHandler(svc))
			r.Delete("/tags/{slug}", api.DeleteTagHandler(svc))
		})
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	c := client.New(config.Host{BaseURL: srv.URL, Token: keyResult.PlaintextKey}, "test")
	return &envBundle{Svc: svc, Server: srv, Client: c, Queries: queries}
}

func TestTags_CreateListDelete(t *testing.T) {
	env := setupTagsEnv(t)

	created, err := env.Client.CreateTag(context.Background(), client.CreateTagParams{
		Slug:        "needs-review",
		DisplayName: "Needs Review",
		Description: "Flagged for human eyes",
	})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if created.Slug != "needs-review" {
		t.Fatalf("slug = %q, want needs-review", created.Slug)
	}

	tags, err := env.Client.ListTags(context.Background())
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	found := false
	for _, tg := range tags {
		if tg.Slug == "needs-review" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created tag not returned by list")
	}

	if err := env.Client.DeleteTag(context.Background(), "needs-review"); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if _, err := env.Client.GetTag(context.Background(), "needs-review"); err == nil {
		t.Fatalf("expected GetTag to fail after delete")
	}
}

func TestTags_UpdateLabel(t *testing.T) {
	env := setupTagsEnv(t)

	if _, err := env.Client.CreateTag(context.Background(), client.CreateTagParams{
		Slug:        "subscription",
		DisplayName: "Subscription",
	}); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	newLabel := "Recurring Subscription"
	updated, err := env.Client.UpdateTag(context.Background(), "subscription", client.UpdateTagParams{
		DisplayName: &newLabel,
	})
	if err != nil {
		t.Fatalf("UpdateTag: %v", err)
	}
	if updated.DisplayName != newLabel {
		t.Fatalf("display_name = %q, want %q", updated.DisplayName, newLabel)
	}
}
