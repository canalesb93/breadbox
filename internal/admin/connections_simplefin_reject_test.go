//go:build !headless && !lite

package admin

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"breadbox/internal/app"
	"breadbox/internal/provider"
)

// SimpleFIN is a Settings-managed singleton bridge: its one connection is
// created/rotated in Settings → Providers, never through the generic
// connect-a-new-connection endpoint. Routing it here would bypass the
// singleton invariant and let a second active SimpleFIN connection be created.
// The handler must reject it with 400 before touching any provider, so the
// invariant has a single enforcement point.
func TestExchangeTokenHandler_RejectsSimpleFIN(t *testing.T) {
	// Empty Providers map: if the reject guard were missing, the request would
	// instead fall through to the "provider not configured" 500 path — so a 400
	// proves the guard fired first.
	a := &app.App{Logger: slog.Default(), Providers: map[string]provider.Provider{}}

	body := `{"public_token":"setup-token","user_id":"00000000-0000-0000-0000-000000000001","institution_id":"simplefin","institution_name":"SimpleFIN","provider":"simplefin"}`
	req := httptest.NewRequest(http.MethodPost, "/-/exchange-token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	ExchangeTokenHandler(a).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (SimpleFIN must be rejected at the connect flow)", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "Settings") {
		t.Errorf("error body should point the user to Settings, got: %s", rec.Body.String())
	}
}
