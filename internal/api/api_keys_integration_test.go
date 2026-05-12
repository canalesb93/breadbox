//go:build integration

package api

import (
	"net/http"
	"strings"
	"testing"
)

// ============================================================
// API Key Management — REST endpoints
// ============================================================

func TestCreateAPIKey_ReturnsPlaintext(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/api-keys", map[string]any{
		"name":  "deploy-bot",
		"scope": "full_access",
	})
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]any
	parseJSON(t, resp, &result)

	plain, _ := result["plaintext_key"].(string)
	if plain == "" {
		t.Fatalf("expected plaintext_key in create response, got %#v", result)
	}
	if !strings.HasPrefix(plain, "bb_") {
		t.Errorf("expected plaintext_key to start with bb_, got %q", plain)
	}
	if result["name"] != "deploy-bot" {
		t.Errorf("expected name deploy-bot, got %v", result["name"])
	}
	if result["scope"] != "full_access" {
		t.Errorf("expected scope full_access, got %v", result["scope"])
	}
}

func TestCreateAPIKey_BadScope(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/api-keys", map[string]any{
		"name":  "bad-scope",
		"scope": "admin",
	})
	readErrorCode(t, resp, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestCreateAPIKey_MissingName(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/api-keys", map[string]any{
		"scope": "full_access",
	})
	readErrorCode(t, resp, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestListAPIKeys_NoPlaintext(t *testing.T) {
	env := setupTestEnv(t)

	// setupTestEnv already created one full_access key. Add another to be sure.
	resp := env.doPost(t, "/api/v1/api-keys", map[string]any{
		"name":  "secondary",
		"scope": "read_only",
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = env.doGet(t, "/api/v1/api-keys")
	assertStatus(t, resp, http.StatusOK)

	var keys []map[string]any
	parseJSON(t, resp, &keys)
	if len(keys) < 2 {
		t.Fatalf("expected at least 2 keys, got %d", len(keys))
	}
	for _, k := range keys {
		if _, ok := k["plaintext_key"]; ok {
			t.Errorf("LIST response leaked plaintext_key for key %q", k["name"])
		}
		if prefix, _ := k["key_prefix"].(string); prefix == "" {
			t.Errorf("expected key_prefix on list response, got %#v", k)
		}
		// The hash itself shouldn't be exposed either (confirm by absence of common field names).
		if _, ok := k["key_hash"]; ok {
			t.Errorf("LIST response leaked key_hash for key %q", k["name"])
		}
	}
}

func TestRevokeAPIKey_Success(t *testing.T) {
	env := setupTestEnv(t)

	// Create a key
	resp := env.doPost(t, "/api/v1/api-keys", map[string]any{
		"name":  "revokable",
		"scope": "full_access",
	})
	assertStatus(t, resp, http.StatusCreated)
	var created map[string]any
	parseJSON(t, resp, &created)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("missing id on created key")
	}

	// Revoke
	resp = env.doDelete(t, "/api/v1/api-keys/"+id)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// List should show it with revoked_at populated
	resp = env.doGet(t, "/api/v1/api-keys")
	assertStatus(t, resp, http.StatusOK)
	var keys []map[string]any
	parseJSON(t, resp, &keys)
	var found bool
	for _, k := range keys {
		if k["id"] == id {
			found = true
			if k["revoked_at"] == nil {
				t.Errorf("expected revoked_at to be set on revoked key, got nil")
			}
		}
	}
	if !found {
		t.Errorf("revoked key not found in list")
	}
}

func TestRevokeAPIKey_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doDelete(t, "/api/v1/api-keys/00000000-0000-0000-0000-000000000001")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestAPIKeyEndpoints_RequireWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)

	// LIST is gated to write — read_only should be blocked
	resp := env.doGet(t, "/api/v1/api-keys")
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")

	// CREATE
	resp = env.doPost(t, "/api/v1/api-keys", map[string]any{"name": "x", "scope": "full_access"})
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")

	// DELETE
	resp = env.doDelete(t, "/api/v1/api-keys/00000000-0000-0000-0000-000000000001")
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

func TestRevokedKey_RejectedByAuth(t *testing.T) {
	env := setupTestEnv(t)

	// Create a fresh key, capture its plaintext.
	resp := env.doPost(t, "/api/v1/api-keys", map[string]any{
		"name":  "soon-revoked",
		"scope": "full_access",
	})
	assertStatus(t, resp, http.StatusCreated)
	var created map[string]any
	parseJSON(t, resp, &created)
	plain, _ := created["plaintext_key"].(string)
	id, _ := created["id"].(string)
	if plain == "" || id == "" {
		t.Fatal("missing plaintext_key/id on create response")
	}

	// Revoke it
	resp = env.doDelete(t, "/api/v1/api-keys/"+id)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// Use the revoked key — should be rejected by auth middleware.
	req, _ := http.NewRequest("GET", env.Server.URL+"/api/v1/users", nil)
	req.Header.Set("X-API-Key", plain)
	got, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	readErrorCode(t, got, http.StatusUnauthorized, "REVOKED_API_KEY")
}

