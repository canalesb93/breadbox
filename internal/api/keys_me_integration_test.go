//go:build integration

package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestWhoami_ReturnsCallerIdentity verifies GET /api/v1/keys/me hands back
// the same actor row that minted the test key. The CLI uses this for
// `breadbox auth whoami`.
func TestWhoami_ReturnsCallerIdentity(t *testing.T) {
	env := setupTestEnv(t)

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/keys/me", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("X-API-Key", env.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("call /keys/me: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}

	var got struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		KeyPrefix string  `json:"key_prefix"`
		Scope     string  `json:"scope"`
		ActorType string  `json:"actor_type"`
		ActorName *string `json:"actor_name"`
		CreatedAt string  `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Name != "test-key" {
		t.Errorf("Name = %q want test-key", got.Name)
	}
	if got.Scope != "full_access" {
		t.Errorf("Scope = %q want full_access", got.Scope)
	}
	// The legacy CreateAPIKeyLegacy call used by setupTestEnv defaults
	// actor_type to "agent" (per the new schema default), so this row
	// should report the default.
	if got.ActorType != "agent" {
		t.Errorf("ActorType = %q want agent (default)", got.ActorType)
	}
	if got.ID == "" {
		t.Error("ID is empty")
	}
}

// TestWhoami_NoAuthReturnsUnauthorized confirms the endpoint is gated
// behind APIKeyAuth — calling without a key returns 401 with the canonical
// envelope.
func TestWhoami_NoAuthReturnsUnauthorized(t *testing.T) {
	env := setupTestEnv(t)

	resp, err := http.Get(env.Server.URL + "/api/v1/keys/me")
	if err != nil {
		t.Fatalf("call /keys/me: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
}
