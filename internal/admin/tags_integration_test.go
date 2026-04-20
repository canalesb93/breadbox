//go:build integration

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

func TestMain(m *testing.M) {
	testutil.RunWithDB(m)
}

// newTestSvc returns a configured service.Service backed by the test DB.
func newTestSvc(t *testing.T) *service.Service {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	return service.New(queries, pool, nil, slog.Default())
}

// TestCreateTagAdmin_Success verifies POST /-/tags creates a tag.
func TestCreateTagAdmin_Success(t *testing.T) {
	svc := newTestSvc(t)
	r := chi.NewRouter()
	r.Post("/-/tags", CreateTagAdminHandler(svc))

	body := []byte(`{"slug":"watchlist","display_name":"Watchlist","color":"#ff0000"}`)
	req := httptest.NewRequest(http.MethodPost, "/-/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp service.TagResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Slug != "watchlist" {
		t.Errorf("expected slug=watchlist, got %s", resp.Slug)
	}
}

// TestCreateTagAdmin_DuplicateSlug verifies the second create returns an error.
func TestCreateTagAdmin_DuplicateSlug(t *testing.T) {
	svc := newTestSvc(t)
	ctx := context.Background()
	if _, err := svc.CreateTag(ctx, service.CreateTagParams{Slug: "duplicate", DisplayName: "Dup"}); err != nil {
		t.Fatalf("seed CreateTag: %v", err)
	}

	r := chi.NewRouter()
	r.Post("/-/tags", CreateTagAdminHandler(svc))
	body := []byte(`{"slug":"duplicate","display_name":"Dup2"}`)
	req := httptest.NewRequest(http.MethodPost, "/-/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusCreated {
		t.Fatalf("expected error response on duplicate slug, got 201")
	}
}

// TestUpdateTagAdmin verifies PUT /-/tags/{id} updates display name + color.
func TestUpdateTagAdmin(t *testing.T) {
	svc := newTestSvc(t)
	ctx := context.Background()
	tag, err := svc.CreateTag(ctx, service.CreateTagParams{Slug: "edit-me", DisplayName: "Original"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	r := chi.NewRouter()
	r.Put("/-/tags/{id}", UpdateTagAdminHandler(svc))

	body := []byte(`{"display_name":"Updated","color":"#123456"}`)
	req := httptest.NewRequest(http.MethodPut, "/-/tags/"+tag.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp service.TagResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.DisplayName != "Updated" {
		t.Errorf("expected display_name='Updated', got %q", resp.DisplayName)
	}
	if resp.Color == nil || *resp.Color != "#123456" {
		t.Errorf("expected color='#123456', got %v", resp.Color)
	}
}

// TestDeleteTagAdmin verifies DELETE /-/tags/{id} removes the tag.
func TestDeleteTagAdmin(t *testing.T) {
	svc := newTestSvc(t)
	ctx := context.Background()
	tag, err := svc.CreateTag(ctx, service.CreateTagParams{Slug: "delete-me", DisplayName: "Doomed"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	r := chi.NewRouter()
	r.Delete("/-/tags/{id}", DeleteTagAdminHandler(svc))

	req := httptest.NewRequest(http.MethodDelete, "/-/tags/"+tag.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if _, err := svc.GetTag(ctx, "delete-me"); err == nil {
		t.Error("expected GetTag to error after deletion")
	}
}

// TestTagsPageRenders is a smoke test that the page handler renders the tag list.
func TestTagsPageRenders(t *testing.T) {
	svc := newTestSvc(t)
	ctx := context.Background()
	if _, err := svc.CreateTag(ctx, service.CreateTagParams{Slug: "needs-review", DisplayName: "Needs Review"}); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	sm := scs.New()
	tr, err := NewTemplateRenderer(sm)
	if err != nil {
		t.Fatalf("NewTemplateRenderer: %v", err)
	}

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	r.Get("/tags", TagsPageHandler(svc, sm, tr))

	req := httptest.NewRequest(http.MethodGet, "/tags", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("needs-review")) {
		t.Error("expected page to mention needs-review tag")
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("Add Tag")) {
		t.Error("expected page to include Add Tag button")
	}
}
