//go:build !lite

package webhook

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"breadbox/internal/provider"

	"github.com/go-chi/chi/v5"
)

// stubProvider records whether HandleWebhook was reached. Only the methods the
// handler touches need real behavior; the rest satisfy the interface.
type stubProvider struct {
	handleCalled bool
	gotBodyLen   int
}

func (s *stubProvider) CreateLinkSession(context.Context, string) (provider.LinkSession, error) {
	return provider.LinkSession{}, nil
}
func (s *stubProvider) ExchangeToken(context.Context, string) (provider.Connection, []provider.Account, error) {
	return provider.Connection{}, nil, nil
}
func (s *stubProvider) SyncTransactions(context.Context, provider.Connection, string) (provider.SyncResult, error) {
	return provider.SyncResult{}, nil
}
func (s *stubProvider) GetBalances(context.Context, provider.Connection) ([]provider.AccountBalance, error) {
	return nil, nil
}
func (s *stubProvider) HandleWebhook(_ context.Context, payload provider.WebhookPayload) (provider.WebhookEvent, error) {
	s.handleCalled = true
	s.gotBodyLen = len(payload.RawBody)
	return provider.WebhookEvent{Type: "unknown"}, nil
}
func (s *stubProvider) CreateReauthSession(context.Context, provider.Connection) (provider.LinkSession, error) {
	return provider.LinkSession{}, nil
}
func (s *stubProvider) RemoveConnection(context.Context, provider.Connection) error { return nil }
func (s *stubProvider) ReconcilesPendingByPolling() bool                            { return false }

// newRequestWithProvider builds a POST carrying the chi "provider" URL param so
// the handler resolves to the registered provider.
func newRequestWithProvider(name string, body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/"+name, bytes.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", name)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// TestWebhookBodyCap verifies that an oversized payload is rejected by the
// MaxBytesReader before the provider's HandleWebhook runs. The oversized path
// returns before any DB work, so nil engine/queries are safe here. (A
// successfully-read body proceeds to a *db.Queries lookup, which can't be
// exercised without a database — that's covered by the integration suite.)
func TestWebhookBodyCap(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	stub := &stubProvider{}
	h := NewHandler(map[string]provider.Provider{"plaid": stub}, nil, nil, logger)

	// One byte over the cap.
	big := bytes.Repeat([]byte("a"), maxWebhookBodyBytes+1)
	rr := httptest.NewRecorder()
	h(rr, newRequestWithProvider("plaid", big))

	if stub.handleCalled {
		t.Fatalf("HandleWebhook was called on an oversized body (read %d bytes); cap not enforced", stub.gotBodyLen)
	}
	// Handler always acknowledges with 200 to avoid provider retries.
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}
