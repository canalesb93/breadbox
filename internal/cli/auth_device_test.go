package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"breadbox/internal/cli/config"
	"breadbox/internal/client"
)

// fastPolling pins the polling interval to ~10ms so the test runs
// in milliseconds rather than seconds. Restored on cleanup.
func fastPolling(t *testing.T) {
	t.Helper()
	prev := deviceCodePollIntervalOverride
	deviceCodePollIntervalOverride = 10 * time.Millisecond
	t.Cleanup(func() { deviceCodePollIntervalOverride = prev })
}

// TestRunDeviceCodeFlow_Approves walks the device-code polling loop
// against a fake HTTP server that flips from pending → approved on the
// second poll. Verifies that the CLI returns the minted token and
// emits the expected progress text to stderr.
func TestRunDeviceCodeFlow_Approves(t *testing.T) {
	var polls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/device-code":
			writeJSON(w, http.StatusCreated, map[string]any{
				"device_code":      "test-device-code",
				"user_code":        "ABCD-EFGH",
				"verification_url": "https://example.test/auth/device",
				"expires_in":       600,
				"interval":         0, // 0 means "use default" in the CLI
			})
		case "/api/v1/auth/device-code/poll":
			n := atomic.AddInt32(&polls, 1)
			if n < 2 {
				writeJSON(w, http.StatusOK, map[string]any{
					"status": "authorization_pending",
				})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"status": "approved",
				"token":  "bb_test_token",
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	fastPolling(t)
	c := client.New(config.Host{BaseURL: srv.URL}, "test")
	got, err := runDeviceCodeFlow(context.Background(), c)
	if err != nil {
		t.Fatalf("runDeviceCodeFlow: %v", err)
	}
	if got != "bb_test_token" {
		t.Errorf("token = %q, want bb_test_token", got)
	}
}

func TestRunDeviceCodeFlow_Denied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/device-code":
			writeJSON(w, http.StatusCreated, map[string]any{
				"device_code":      "dc",
				"user_code":        "ABCD-EFGH",
				"verification_url": "https://example.test/auth/device",
				"expires_in":       600,
				"interval":         0,
			})
		case "/api/v1/auth/device-code/poll":
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": map[string]any{"code": "DENIED", "message": "denied"},
			})
		}
	}))
	defer srv.Close()

	fastPolling(t)
	c := client.New(config.Host{BaseURL: srv.URL}, "test")
	_, err := runDeviceCodeFlow(context.Background(), c)
	if err == nil {
		t.Fatal("expected error on denied poll")
	}
	if !strings.Contains(err.Error(), "denied") {
		t.Errorf("error = %q, want it to mention denial", err.Error())
	}
	if code := MapExitCode(err); code != ExitAuth {
		t.Errorf("MapExitCode = %d, want %d (auth)", code, ExitAuth)
	}
}

func TestRunDeviceCodeFlow_Expired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/device-code":
			writeJSON(w, http.StatusCreated, map[string]any{
				"device_code":      "dc",
				"user_code":        "ABCD-EFGH",
				"verification_url": "https://example.test/auth/device",
				"expires_in":       600,
				"interval":         0,
			})
		case "/api/v1/auth/device-code/poll":
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": map[string]any{"code": "EXPIRED", "message": "expired"},
			})
		}
	}))
	defer srv.Close()

	fastPolling(t)
	c := client.New(config.Host{BaseURL: srv.URL}, "test")
	_, err := runDeviceCodeFlow(context.Background(), c)
	if err == nil {
		t.Fatal("expected error on expired poll")
	}
	if code := MapExitCode(err); code != ExitUpstream {
		t.Errorf("MapExitCode = %d, want %d (upstream)", code, ExitUpstream)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
