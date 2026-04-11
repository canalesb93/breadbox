package teller

import (
	"errors"
	"net/http"
	"testing"

	"breadbox/internal/provider"
)

func TestClassifyHTTPError_ServerAndRateLimitErrors_NoSyncRetryable(t *testing.T) {
	// Server errors (5xx) and rate limits (429) must NOT wrap ErrSyncRetryable.
	// Teller retry is handled by DoWithRetry at the HTTP call site, not by the
	// sync engine's cursor-reset loop (which is only for Plaid's mutation-during-pagination).
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantSubstr string
	}{
		{
			name:       "500 internal server error",
			statusCode: http.StatusInternalServerError,
			body:       "internal error",
			wantSubstr: "server error (status 500)",
		},
		{
			name:       "502 bad gateway",
			statusCode: http.StatusBadGateway,
			body:       "bad gateway",
			wantSubstr: "server error (status 502)",
		},
		{
			name:       "503 service unavailable",
			statusCode: http.StatusServiceUnavailable,
			body:       "service unavailable",
			wantSubstr: "server error (status 503)",
		},
		{
			name:       "429 rate limited",
			statusCode: http.StatusTooManyRequests,
			body:       "",
			wantSubstr: "rate limited (status 429)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyHTTPError("test op", tt.statusCode, []byte(tt.body))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if errors.Is(err, provider.ErrSyncRetryable) {
				t.Errorf("errors.Is(err, ErrSyncRetryable) = true for status %d, want false", tt.statusCode)
			}
			if errors.Is(err, provider.ErrReauthRequired) {
				t.Errorf("errors.Is(err, ErrReauthRequired) = true for status %d, want false", tt.statusCode)
			}
			if !containsSubstring(err.Error(), tt.wantSubstr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}

func TestClassifyHTTPError_ClientErrors_NoSentinel(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "400 bad request", statusCode: http.StatusBadRequest},
		{name: "401 unauthorized", statusCode: http.StatusUnauthorized},
		{name: "404 not found", statusCode: http.StatusNotFound},
		{name: "422 unprocessable", statusCode: http.StatusUnprocessableEntity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyHTTPError("test op", tt.statusCode, []byte("error body"))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if errors.Is(err, provider.ErrSyncRetryable) {
				t.Errorf("errors.Is(err, ErrSyncRetryable) = true for status %d, want false", tt.statusCode)
			}
			if errors.Is(err, provider.ErrReauthRequired) {
				t.Errorf("errors.Is(err, ErrReauthRequired) = true for status %d, want false", tt.statusCode)
			}
		})
	}
}

func TestErrReauthRequired_WrapsProviderSentinel(t *testing.T) {
	if !errors.Is(ErrReauthRequired, provider.ErrReauthRequired) {
		t.Errorf("errors.Is(teller.ErrReauthRequired, provider.ErrReauthRequired) = false, want true")
	}
}

func TestClassifyHTTPError_ContainsOperationContext(t *testing.T) {
	err := classifyHTTPError("balance get", http.StatusInternalServerError, []byte("oops"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !containsSubstring(msg, "balance get") {
		t.Errorf("error message %q does not contain operation name 'balance get'", msg)
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
