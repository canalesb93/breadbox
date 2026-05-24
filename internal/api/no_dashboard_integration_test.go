//go:build integration && !lite

package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"breadbox/internal/app"
	"breadbox/internal/config"
	bsync "breadbox/internal/sync"
	"breadbox/internal/testutil"
)

// TestNoDashboard_GatesAdminButKeepsAPIAndDiscovery exercises the runtime
// --no-dashboard switch. We build a real chi router via api.NewRouter so the
// gating logic in router.go is what gets tested. The dashboard surface must
// 404; the REST API + OAuth discovery + version endpoints must stay up.
func TestNoDashboard_GatesAdminButKeepsAPIAndDiscovery(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	engine := bsync.NewEngine(queries, pool, nil, slog.Default())

	cfg := &config.Config{
		ServerPort:        "0",
		Environment:       "local",
		NoDashboard:       true,
		APIRateLimitRPM:   120,
		APIRateLimitBurst: 60,
	}
	a := &app.App{
		DB:         pool,
		Queries:    queries,
		Config:     cfg,
		Logger:     slog.Default(),
		SyncEngine: engine,
		Providers:  nil,
	}

	// "dev" version short-circuits VersionHandler's GitHub probe (which
	// would otherwise NPE on the nil VersionChecker — populated only by
	// runServe). The endpoint still returns 200 with update_available=false.
	router := NewRouter(a, "dev")
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	cases := []struct {
		name     string
		path     string
		wantCode int
	}{
		// REST + meta — must stay up.
		{"version", "/api/v1/version", http.StatusOK},
		{"health_live", "/health/live", http.StatusOK},
		// OAuth 2.1 discovery — MCP clients depend on this.
		{"oauth_metadata", "/.well-known/oauth-authorization-server", http.StatusOK},
		{"oauth_resource", "/.well-known/oauth-protected-resource", http.StatusOK},
		// Dashboard — must be gone.
		{"admin_root", "/", http.StatusNotFound},
		{"web_v1_me", "/web/v1/me", http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(srv.URL + tc.path)
			if err != nil {
				t.Fatalf("get %s: %v", tc.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantCode {
				t.Errorf("%s: status got %d, want %d", tc.path, resp.StatusCode, tc.wantCode)
			}
		})
	}
}
