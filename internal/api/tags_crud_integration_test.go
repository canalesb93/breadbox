//go:build integration

// Integration tests for the tag CRUD endpoints (POST /tags, GET/PATCH/DELETE
// /tags/{slug}). The {slug} URL param accepts UUID, short_id, or slug — see
// service.GetTag.
//
// Run with: DATABASE_URL=... go test -tags integration -count=1 -p 1 -v -run Tag ./internal/api/...

package api

import (
	"net/http"
	"testing"

	"breadbox/internal/service"
)

// --- helpers ---------------------------------------------------------------

// createTagVia POSTs /api/v1/tags and decodes the resulting TagResponse. Fails
// the test on non-201.
func createTagVia(t *testing.T, env *testEnv, body map[string]any) service.TagResponse {
	t.Helper()
	resp := env.doPost(t, "/api/v1/tags", body)
	assertStatus(t, resp, http.StatusCreated)
	var tag service.TagResponse
	parseJSON(t, resp, &tag)
	return tag
}

// --- POST /tags ------------------------------------------------------------

func TestCreateTag_Success(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/tags", map[string]any{
		"slug":         "needs-review",
		"display_name": "Needs Review",
		"description":  "Triage queue",
		"color":        "#abc123",
		"icon":         "tag",
	})
	assertStatus(t, resp, http.StatusCreated)

	var tag service.TagResponse
	parseJSON(t, resp, &tag)

	if tag.Slug != "needs-review" {
		t.Errorf("slug: got %q, want needs-review", tag.Slug)
	}
	if tag.DisplayName != "Needs Review" {
		t.Errorf("display_name: got %q, want Needs Review", tag.DisplayName)
	}
	if tag.Description != "Triage queue" {
		t.Errorf("description: got %q, want Triage queue", tag.Description)
	}
	if tag.Color == nil || *tag.Color != "#abc123" {
		t.Errorf("color: got %v, want #abc123", tag.Color)
	}
	if tag.Icon == nil || *tag.Icon != "tag" {
		t.Errorf("icon: got %v, want tag", tag.Icon)
	}
	if tag.ShortID == "" {
		t.Errorf("short_id should be set")
	}
	if tag.ID == "" {
		t.Errorf("id should be set")
	}
	if tag.Lifecycle != "persistent" {
		t.Errorf("lifecycle default: got %q, want persistent", tag.Lifecycle)
	}
}

func TestCreateTag_DuplicateSlug(t *testing.T) {
	env := setupTestEnv(t)

	_ = createTagVia(t, env, map[string]any{
		"slug":         "duplicate-slug",
		"display_name": "First",
	})
	resp := env.doPost(t, "/api/v1/tags", map[string]any{
		"slug":         "duplicate-slug",
		"display_name": "Second",
	})
	readErrorCode(t, resp, http.StatusConflict, "SLUG_CONFLICT")
}

func TestCreateTag_InvalidSlug(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/tags", map[string]any{
		"slug":         "Invalid Slug!", // spaces + uppercase + bang
		"display_name": "Bad",
	})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestCreateTag_EmptyDisplayName(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/tags", map[string]any{
		"slug":         "no-name",
		"display_name": "   ", // trims to empty
	})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

// --- GET /tags/{slug} ------------------------------------------------------

func TestGetTag_BySlug(t *testing.T) {
	env := setupTestEnv(t)
	created := createTagVia(t, env, map[string]any{
		"slug":         "by-slug",
		"display_name": "By Slug",
	})

	resp := env.doGet(t, "/api/v1/tags/by-slug")
	assertStatus(t, resp, http.StatusOK)

	var got service.TagResponse
	parseJSON(t, resp, &got)
	if got.ID != created.ID {
		t.Errorf("id mismatch: got %s, want %s", got.ID, created.ID)
	}
}

func TestGetTag_ByShortID(t *testing.T) {
	env := setupTestEnv(t)
	created := createTagVia(t, env, map[string]any{
		"slug":         "by-short",
		"display_name": "By Short",
	})

	resp := env.doGet(t, "/api/v1/tags/"+created.ShortID)
	assertStatus(t, resp, http.StatusOK)

	var got service.TagResponse
	parseJSON(t, resp, &got)
	if got.Slug != "by-short" {
		t.Errorf("slug: got %q, want by-short", got.Slug)
	}
}

func TestGetTag_ByUUID(t *testing.T) {
	env := setupTestEnv(t)
	created := createTagVia(t, env, map[string]any{
		"slug":         "by-uuid",
		"display_name": "By UUID",
	})

	resp := env.doGet(t, "/api/v1/tags/"+created.ID)
	assertStatus(t, resp, http.StatusOK)

	var got service.TagResponse
	parseJSON(t, resp, &got)
	if got.Slug != "by-uuid" {
		t.Errorf("slug: got %q, want by-uuid", got.Slug)
	}
}

func TestGetTag_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doGet(t, "/api/v1/tags/no-such-tag")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// --- PATCH /tags/{slug} ----------------------------------------------------

func TestUpdateTag_PatchPartial(t *testing.T) {
	env := setupTestEnv(t)

	color := "#111111"
	icon := "old-icon"
	created := createTagVia(t, env, map[string]any{
		"slug":         "patch-me",
		"display_name": "Original",
		"description":  "original desc",
		"color":        color,
		"icon":         icon,
	})

	// Only update display_name; everything else should stay.
	resp := env.doPatch(t, "/api/v1/tags/patch-me", map[string]any{
		"display_name": "Updated",
	})
	assertStatus(t, resp, http.StatusOK)

	var got service.TagResponse
	parseJSON(t, resp, &got)

	if got.DisplayName != "Updated" {
		t.Errorf("display_name: got %q, want Updated", got.DisplayName)
	}
	if got.Description != created.Description {
		t.Errorf("description should be preserved: got %q, want %q", got.Description, created.Description)
	}
	if got.Color == nil || *got.Color != color {
		t.Errorf("color should be preserved: got %v, want %s", got.Color, color)
	}
	if got.Icon == nil || *got.Icon != icon {
		t.Errorf("icon should be preserved: got %v, want %s", got.Icon, icon)
	}
	if got.Lifecycle != created.Lifecycle {
		t.Errorf("lifecycle should be preserved: got %q, want %q", got.Lifecycle, created.Lifecycle)
	}
}

func TestUpdateTag_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doPatch(t, "/api/v1/tags/no-such-tag", map[string]any{
		"display_name": "Whatever",
	})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestUpdateTag_EmptyDisplayName(t *testing.T) {
	env := setupTestEnv(t)
	_ = createTagVia(t, env, map[string]any{
		"slug":         "patch-empty",
		"display_name": "Original",
	})

	resp := env.doPatch(t, "/api/v1/tags/patch-empty", map[string]any{
		"display_name": "   ", // trims to empty
	})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

// --- DELETE /tags/{slug} ---------------------------------------------------

func TestDeleteTag_Success(t *testing.T) {
	env := setupTestEnv(t)
	_ = createTagVia(t, env, map[string]any{
		"slug":         "delete-me",
		"display_name": "Delete Me",
	})

	resp := env.doDelete(t, "/api/v1/tags/delete-me")
	assertStatus(t, resp, http.StatusNoContent)

	// Subsequent GET should 404.
	getResp := env.doGet(t, "/api/v1/tags/delete-me")
	readErrorCode(t, getResp, http.StatusNotFound, "NOT_FOUND")
}

func TestDeleteTag_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doDelete(t, "/api/v1/tags/no-such-tag")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// --- Scope enforcement -----------------------------------------------------

func TestCreateTag_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	resp := env.doPost(t, "/api/v1/tags", map[string]any{
		"slug":         "ro-create",
		"display_name": "Nope",
	})
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

func TestPatchTag_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	resp := env.doPatch(t, "/api/v1/tags/anything", map[string]any{
		"display_name": "Nope",
	})
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

func TestDeleteTag_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	resp := env.doDelete(t, "/api/v1/tags/anything")
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

func TestGetTag_AllowsReadScope(t *testing.T) {
	// Read-only key should be able to GET. Seed via a full-access env first,
	// then re-issue the GET with a read-only key on a fresh server.
	env := setupTestEnv(t)
	_ = createTagVia(t, env, map[string]any{
		"slug":         "read-me",
		"display_name": "Read Me",
	})

	// Same DB, new env with read-only key.
	roKey, err := env.Service.CreateAPIKeyLegacy(t.Context(), "ro-read-tag", "read_only")
	if err != nil {
		t.Fatalf("create read-only API key: %v", err)
	}
	roEnv := &testEnv{Server: env.Server, APIKey: roKey.PlaintextKey, Service: env.Service, Queries: env.Queries, Pool: env.Pool}

	resp := roEnv.doGet(t, "/api/v1/tags/read-me")
	assertStatus(t, resp, http.StatusOK)
}
