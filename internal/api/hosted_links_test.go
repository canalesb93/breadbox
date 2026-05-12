//go:build integration

package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"breadbox/internal/testutil"
)

// hostedLinkResponseBody mirrors the JSON shape of createHostedLinkResponse
// so individual tests can introspect it without re-declaring fields.
type hostedLinkResponseBody struct {
	ID                  string   `json:"id"`
	ShortID             string   `json:"short_id"`
	UserID              string   `json:"user_id"`
	Provider            string   `json:"provider"`
	Action              string   `json:"action"`
	SingleUse           bool     `json:"single_use"`
	RedirectURL         string   `json:"redirect_url"`
	Label               string   `json:"label"`
	Status              string   `json:"status"`
	ResultConnectionIDs []string `json:"result_connection_ids"`
	ExpiresAt           string   `json:"expires_at"`
	CreatedAt           string   `json:"created_at"`
	Token               string   `json:"token"`
	URL                 string   `json:"url"`
}

func TestHostedLink_Create_HappyPath(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPost(t, "/api/v1/connections/link", map[string]any{
		"user_id":            user.ShortID,
		"provider":           "plaid",
		"label":              "Chase checking",
		"expires_in_seconds": 300,
	})
	assertStatus(t, resp, http.StatusCreated)

	var body hostedLinkResponseBody
	parseJSON(t, resp, &body)
	if body.Token == "" {
		t.Fatal("expected non-empty token on create response")
	}
	if body.URL == "" || !strings.Contains(body.URL, "/link/"+body.Token) {
		t.Fatalf("expected url to end with /link/<token>, got %q", body.URL)
	}
	// httptest.Server uses an HTTP scheme — confirm the URL reflects the
	// request's actual host:port so the test server URL is the prefix.
	if !strings.HasPrefix(body.URL, env.Server.URL+"/link/") {
		t.Fatalf("expected url to start with %s/link/, got %q", env.Server.URL, body.URL)
	}
	if body.Status != "pending" {
		t.Errorf("expected status=pending, got %q", body.Status)
	}
	if body.Action != "link" {
		t.Errorf("expected action=link, got %q", body.Action)
	}
	if body.Provider != "plaid" {
		t.Errorf("expected provider=plaid, got %q", body.Provider)
	}
	if body.Label != "Chase checking" {
		t.Errorf("expected label to round-trip, got %q", body.Label)
	}
	if body.ResultConnectionIDs == nil {
		t.Error("expected result_connection_ids to be a present empty array, got nil")
	}
}

func TestHostedLink_Create_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPost(t, "/api/v1/connections/link", map[string]any{
		"user_id":  user.ShortID,
		"provider": "plaid",
	})
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

func TestHostedLink_Create_InvalidProvider(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPost(t, "/api/v1/connections/link", map[string]any{
		"user_id":  user.ShortID,
		"provider": "bogus",
	})
	readErrorCode(t, resp, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestHostedLink_Create_InvalidExpiry(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	// Negative.
	resp := env.doPost(t, "/api/v1/connections/link", map[string]any{
		"user_id":            user.ShortID,
		"expires_in_seconds": -5,
	})
	readErrorCode(t, resp, http.StatusBadRequest, "VALIDATION_ERROR")

	// Above the 1h ceiling.
	resp = env.doPost(t, "/api/v1/connections/link", map[string]any{
		"user_id":            user.ShortID,
		"expires_in_seconds": 3601,
	})
	readErrorCode(t, resp, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestHostedLink_Get_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/connections/link/aaaaaaaa")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestHostedLink_Get_OmitsToken(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	created := env.doPost(t, "/api/v1/connections/link", map[string]any{
		"user_id":  user.ShortID,
		"provider": "plaid",
	})
	assertStatus(t, created, http.StatusCreated)
	var createdBody hostedLinkResponseBody
	parseJSON(t, created, &createdBody)
	if createdBody.Token == "" {
		t.Fatal("setup: expected token on create response")
	}

	resp := env.doGet(t, "/api/v1/connections/link/"+createdBody.ShortID)
	assertStatus(t, resp, http.StatusOK)

	// Decode into a raw map so an unexpectedly-present `token` field surfaces
	// regardless of how the typed struct strips it.
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["token"]; ok {
		t.Errorf("Get response unexpectedly carries `token` field: %s", string(data))
	}
	if _, ok := raw["url"]; ok {
		t.Errorf("Get response unexpectedly carries `url` field: %s", string(data))
	}
	if _, ok := raw["result_connection_ids"]; !ok {
		t.Errorf("Get response missing `result_connection_ids`: %s", string(data))
	}
}

func TestHostedLink_Get_ByShortID(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	created := env.doPost(t, "/api/v1/connections/link", map[string]any{
		"user_id":  user.ShortID,
		"provider": "teller",
	})
	assertStatus(t, created, http.StatusCreated)
	var createdBody hostedLinkResponseBody
	parseJSON(t, created, &createdBody)

	// Look up by both UUID and short_id; both paths must resolve.
	respUUID := env.doGet(t, "/api/v1/connections/link/"+createdBody.ID)
	assertStatus(t, respUUID, http.StatusOK)
	respUUID.Body.Close()

	respShort := env.doGet(t, "/api/v1/connections/link/"+createdBody.ShortID)
	assertStatus(t, respShort, http.StatusOK)
	var got hostedLinkResponseBody
	parseJSON(t, respShort, &got)
	if got.ID != createdBody.ID || got.ShortID != createdBody.ShortID {
		t.Errorf("short_id lookup returned wrong session: want id=%s short=%s, got id=%s short=%s",
			createdBody.ID, createdBody.ShortID, got.ID, got.ShortID)
	}
	if got.Provider != "teller" {
		t.Errorf("expected provider=teller, got %q", got.Provider)
	}
}
