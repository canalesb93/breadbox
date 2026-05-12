//go:build integration

// Integration tests for the categories noun group. Drives the real REST
// handlers via an in-process httptest server.
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

func setupCatEnv(t *testing.T) *envBundle {
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
		r.Get("/categories", api.ListCategoriesHandler(svc))
		r.Get("/categories/export", api.ExportCategoriesTSVHandler(svc))
		r.Get("/categories/{id}", api.GetCategoryHandler(svc))
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Post("/categories", api.CreateCategoryHandler(svc))
			r.Post("/categories/import", api.ImportCategoriesTSVHandler(svc))
			r.Put("/categories/{id}", api.UpdateCategoryHandler(svc))
			r.Delete("/categories/{id}", api.DeleteCategoryHandler(svc))
			r.Post("/categories/{id}/merge", api.MergeCategoriesHandler(svc))
		})
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	c := client.New(config.Host{BaseURL: srv.URL, Token: keyResult.PlaintextKey}, "test")
	return &envBundle{Svc: svc, Server: srv, Client: c, Queries: queries}
}

func TestCategories_ListReturnsFixtures(t *testing.T) {
	env := setupCatEnv(t)
	testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")

	cats, err := env.Client.ListCategories(context.Background())
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(cats) == 0 {
		t.Fatalf("expected at least one category, got 0")
	}
}

func TestCategories_CreateMergeDelete(t *testing.T) {
	env := setupCatEnv(t)

	// The seed migration's uncategorized row is dropped by the truncate
	// between tests; the DELETE handler reassigns orphaned transactions to
	// it, so we recreate it for this test.
	testutil.MustCreateCategory(t, env.Queries, "uncategorized", "Uncategorized")

	src, err := env.Client.CreateCategory(context.Background(), client.CreateCategoryParams{
		DisplayName: "Source",
		Slug:        "source-cli",
	})
	if err != nil {
		t.Fatalf("CreateCategory source: %v", err)
	}
	tgt, err := env.Client.CreateCategory(context.Background(), client.CreateCategoryParams{
		DisplayName: "Target",
		Slug:        "target-cli",
	})
	if err != nil {
		t.Fatalf("CreateCategory target: %v", err)
	}

	if err := env.Client.MergeCategories(context.Background(), src.ID, tgt.ID); err != nil {
		t.Fatalf("MergeCategories: %v", err)
	}

	// After merge, the source row is gone.
	if _, err := env.Client.GetCategory(context.Background(), src.ID); err == nil {
		t.Fatalf("expected merged source to be deleted, but Get succeeded")
	}
	// Target survives the merge.
	if _, err := env.Client.GetCategory(context.Background(), tgt.ID); err != nil {
		t.Fatalf("GetCategory target after merge: %v", err)
	}
}
