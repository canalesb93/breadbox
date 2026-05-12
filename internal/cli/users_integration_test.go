//go:build integration

// Integration tests for the users / logins / keys noun groups in the CLI's
// REST client. Mirrors internal/api/api_integration_test.go's infra: a chi
// router wired with API key auth runs in an httptest.Server; the
// internal/client package talks to it through `client.New`. The cobra
// command tree is exercised at the validation seam in keys_test.go (unit);
// here we verify the wire round-trips.
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
	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
	bsync "breadbox/internal/sync"
	"breadbox/internal/testutil"

	"github.com/go-chi/chi/v5"
)

func TestMain(m *testing.M) {
	testutil.RunWithDB(m)
}

// testServer is the shared test fixture: chi router + http server + a
// configured client tied to a full_access key.
type testServer struct {
	t       *testing.T
	server  *httptest.Server
	service *service.Service
	client  *client.Client
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	engine := bsync.NewEngine(queries, pool, nil, slog.Default())
	svc := service.New(queries, pool, engine, slog.Default())

	keyResult, err := svc.CreateAPIKeyLegacy(t.Context(), "test-key", "full_access")
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))
		r.Get("/users", api.ListUsersHandler(svc))
		r.Get("/users/{id}", api.GetUserHandler(svc))
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Post("/users", api.CreateUserHandler(svc))
			r.Patch("/users/{id}", api.UpdateUserHandler(svc))
			r.Delete("/users/{id}", api.DeleteUserHandler(svc))
			r.Get("/users/{user_id}/login", api.ListUserLoginsHandler(svc))
			r.Post("/users/{user_id}/login", api.CreateUserLoginHandler(svc))
			r.Patch("/users/{user_id}/login/{login_id}", api.UpdateUserLoginHandler(svc))
			r.Delete("/users/{user_id}/login/{login_id}", api.DeleteUserLoginHandler(svc))
			r.Post("/users/{user_id}/login/{login_id}/regenerate-token", api.RegenerateLoginTokenHandler(svc))
			r.Get("/login-accounts", api.ListLoginAccountsHandler(svc))
			r.Delete("/login-accounts/{id}", api.DeleteLoginAccountHandler(svc))
			r.Post("/login-accounts/{id}/reset-password", api.ResetLoginAccountPasswordHandler(svc))
			r.Get("/api-keys", api.ListAPIKeysHandler(svc))
			r.Post("/api-keys", api.CreateAPIKeyHandler(svc))
			r.Delete("/api-keys/{id}", api.RevokeAPIKeyHandler(svc))
		})
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	host := config.Host{BaseURL: srv.URL, Token: keyResult.PlaintextKey}
	c := client.New(host, "test")
	return &testServer{t: t, server: srv, service: svc, client: c}
}

// ============================================================
// users
// ============================================================

func TestUsers_CreateGetUpdateDelete(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()
	email := "alice@example.com"

	created, err := ts.client.CreateUser(ctx, client.CreateUserRequest{Name: "Alice", Email: &email})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.Name != "Alice" {
		t.Fatalf("created.Name = %q, want Alice", created.Name)
	}
	if created.Email == nil || *created.Email != email {
		t.Fatalf("created.Email = %v, want %s", created.Email, email)
	}
	if created.ShortID == "" {
		t.Fatalf("created.ShortID empty")
	}

	got, err := ts.client.GetUser(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetUser by uuid: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("get returned different id: %s vs %s", got.ID, created.ID)
	}

	// Round-trip by short_id too — exercises the resolve layer.
	bySID, err := ts.client.GetUser(ctx, created.ShortID)
	if err != nil {
		t.Fatalf("GetUser by short_id: %v", err)
	}
	if bySID.ID != created.ID {
		t.Fatalf("short_id lookup returned different id")
	}

	newName := "Alicia"
	updated, err := ts.client.UpdateUser(ctx, created.ID, client.UpdateUserRequest{Name: &newName})
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if updated.Name != "Alicia" {
		t.Fatalf("updated.Name = %q", updated.Name)
	}

	if err := ts.client.DeleteUser(ctx, created.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	// After delete, GET should 404.
	if _, err := ts.client.GetUser(ctx, created.ID); err == nil {
		t.Fatalf("GetUser after delete: expected error, got nil")
	} else {
		apiErr, ok := err.(*client.APIError)
		if !ok {
			t.Fatalf("expected *APIError, got %T: %v", err, err)
		}
		if apiErr.Status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", apiErr.Status)
		}
	}
}

// ============================================================
// logins
// ============================================================

func TestLogins_CreateListResetDelete(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()

	user, err := ts.client.CreateUser(ctx, client.CreateUserRequest{Name: "Bob"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	login, err := ts.client.CreateLoginAccount(ctx, user.ID, client.CreateLoginRequest{
		Username: "bob@example.com",
		Role:     "admin",
	})
	if err != nil {
		t.Fatalf("CreateLoginAccount: %v", err)
	}
	if login.SetupToken == "" {
		t.Fatalf("setup token must be present on create response")
	}
	if !strings.EqualFold(login.Username, "bob@example.com") {
		t.Fatalf("username = %q", login.Username)
	}

	// List the top-level surface — the new endpoint added in this PR.
	all, err := ts.client.ListLoginAccounts(ctx)
	if err != nil {
		t.Fatalf("ListLoginAccounts: %v", err)
	}
	var found bool
	for _, l := range all {
		if l.ID == login.ID {
			found = true
			// setup_token MUST NOT appear in the list response.
			if l.SetupToken != "" {
				t.Fatalf("setup_token leaked into list response: %q", l.SetupToken)
			}
		}
	}
	if !found {
		t.Fatalf("created login not found in list")
	}

	// Reset password produces a fresh token.
	resp, err := ts.client.ResetLoginPassword(ctx, login.ID)
	if err != nil {
		t.Fatalf("ResetLoginPassword: %v", err)
	}
	if resp.SetupToken == "" {
		t.Fatalf("reset-password setup_token empty")
	}
	if resp.SetupToken == login.SetupToken {
		t.Fatalf("reset-password returned the same token (no-op rotation?)")
	}

	// Delete by login id alone (no parent user_id required).
	if err := ts.client.DeleteLoginAccount(ctx, login.ID); err != nil {
		t.Fatalf("DeleteLoginAccount: %v", err)
	}

	all2, err := ts.client.ListLoginAccounts(ctx)
	if err != nil {
		t.Fatalf("ListLoginAccounts after delete: %v", err)
	}
	for _, l := range all2 {
		if l.ID == login.ID {
			t.Fatalf("login still present after delete")
		}
	}
}

// ============================================================
// keys
// ============================================================

func TestKeys_CreateListRevoke(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()

	result, err := ts.client.CreateAPIKey(ctx, client.CreateAPIKeyRequest{
		Name:      "test-agent",
		Scope:     "full_access",
		ActorType: "agent",
		ActorName: "test-agent",
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if !strings.HasPrefix(result.PlaintextKey, "bb_") {
		t.Fatalf("plaintext key missing bb_ prefix: %q", result.PlaintextKey)
	}
	if result.ActorType != "agent" {
		t.Fatalf("actor_type = %q, want agent", result.ActorType)
	}
	if result.ActorName == nil || *result.ActorName != "test-agent" {
		t.Fatalf("actor_name = %v, want test-agent", result.ActorName)
	}

	keys, err := ts.client.ListAPIKeys(ctx)
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	var found bool
	for _, k := range keys {
		if k.ID == result.ID {
			found = true
			if k.KeyPrefix != result.KeyPrefix {
				t.Fatalf("list key_prefix mismatch")
			}
			if k.ActorType != "agent" {
				t.Fatalf("list actor_type = %q", k.ActorType)
			}
		}
	}
	if !found {
		t.Fatalf("created key not present in list")
	}

	// Revoke — afterwards, requests using the new key should fail with 401.
	if err := ts.client.RevokeAPIKey(ctx, result.ID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}

	revokedClient := client.New(config.Host{BaseURL: ts.server.URL, Token: result.PlaintextKey}, "test")
	_, err = revokedClient.ListAPIKeys(ctx)
	if err == nil {
		t.Fatalf("expected revoked key to be rejected, got nil error")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *APIError after revoke, got %T: %v", err, err)
	}
	if apiErr.Status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", apiErr.Status)
	}
}
