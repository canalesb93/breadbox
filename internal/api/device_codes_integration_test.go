//go:build integration

package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"breadbox/internal/api"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/go-chi/chi/v5"
)

// newDeviceCodeServer wires just the two unauthenticated device-code
// endpoints. APIKeyAuth is deliberately absent so the test exercises
// the same surface a remote CLI talks to before any key exists.
func newDeviceCodeServer(t *testing.T, svc *service.Service) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	r.Post("/api/v1/auth/device-code", api.CreateDeviceCodeHandler(svc))
	r.Post("/api/v1/auth/device-code/poll", api.PollDeviceCodeHandler(svc))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func postJSON(t *testing.T, url string, body any) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	resp, err := http.Post(url, "application/json", &buf)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func TestDeviceCodeEndpoints_PendingThenApproved(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())
	srv := newDeviceCodeServer(t, svc)

	// 1. Initiate.
	status, body := postJSON(t, srv.URL+"/api/v1/auth/device-code", struct{}{})
	if status != http.StatusCreated {
		t.Fatalf("initiate status = %d, want 201; body = %s", status, body)
	}
	var init struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURL string `json:"verification_url"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := json.Unmarshal(body, &init); err != nil {
		t.Fatalf("decode initiate: %v; body = %s", err, body)
	}
	if init.DeviceCode == "" || !strings.Contains(init.UserCode, "-") {
		t.Fatalf("unexpected initiate body: %+v", init)
	}

	// 2. Poll before approval.
	status, body = postJSON(t, srv.URL+"/api/v1/auth/device-code/poll", map[string]string{
		"device_code": init.DeviceCode,
	})
	if status != http.StatusOK {
		t.Fatalf("pending poll status = %d, body = %s", status, body)
	}
	var pending struct {
		Status string `json:"status"`
		Token  string `json:"token"`
	}
	_ = json.Unmarshal(body, &pending)
	if pending.Status != "authorization_pending" {
		t.Errorf("pending status = %q, want authorization_pending", pending.Status)
	}
	if pending.Token != "" {
		t.Errorf("pending poll leaked token: %q", pending.Token)
	}

	// 3. Approve via service layer (skipping the browser).
	if _, err := svc.ApproveDeviceCode(t.Context(), service.ApproveDeviceCodeParams{
		UserCode:  init.UserCode,
		ActorName: "integration-test",
	}); err != nil {
		t.Fatalf("ApproveDeviceCode: %v", err)
	}

	// 4. Poll again — token comes back.
	status, body = postJSON(t, srv.URL+"/api/v1/auth/device-code/poll", map[string]string{
		"device_code": init.DeviceCode,
	})
	if status != http.StatusOK {
		t.Fatalf("approved poll status = %d, body = %s", status, body)
	}
	var approved struct {
		Status string `json:"status"`
		Token  string `json:"token"`
	}
	_ = json.Unmarshal(body, &approved)
	if approved.Status != "approved" {
		t.Errorf("approved status = %q, want approved", approved.Status)
	}
	if !strings.HasPrefix(approved.Token, "bb_") {
		t.Errorf("token = %q, want bb_ prefix", approved.Token)
	}
}

func TestDeviceCodeEndpoints_PollInvalidCode(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())
	srv := newDeviceCodeServer(t, svc)

	status, body := postJSON(t, srv.URL+"/api/v1/auth/device-code/poll", map[string]string{
		"device_code": "completely-fake-token",
	})
	if status != http.StatusNotFound {
		t.Fatalf("unknown poll status = %d, want 404; body = %s", status, body)
	}
	if !strings.Contains(string(body), "INVALID_DEVICE_CODE") {
		t.Errorf("body missing INVALID_DEVICE_CODE: %s", body)
	}
}

func TestDeviceCodeEndpoints_PollDenied(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())
	srv := newDeviceCodeServer(t, svc)

	dc, err := svc.CreateDeviceCode(t.Context())
	if err != nil {
		t.Fatalf("CreateDeviceCode: %v", err)
	}
	if err := svc.DenyDeviceCode(t.Context(), dc.UserCode, ""); err != nil {
		t.Fatalf("DenyDeviceCode: %v", err)
	}

	status, body := postJSON(t, srv.URL+"/api/v1/auth/device-code/poll", map[string]string{
		"device_code": dc.DeviceCode,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("denied poll status = %d, want 400; body = %s", status, body)
	}
	if !strings.Contains(string(body), "DENIED") {
		t.Errorf("body missing DENIED code: %s", body)
	}
}
