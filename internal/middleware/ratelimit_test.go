//go:build !lite

package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// makeAPIKeyCtx builds a context carrying an API key with a deterministic
// UUID so tests can verify per-key isolation without hitting the database.
func makeAPIKeyCtx(t *testing.T, suffix byte) context.Context {
	t.Helper()
	var b [16]byte
	for i := range b {
		b[i] = suffix
	}
	key := &db.ApiKey{
		ID:    pgtype.UUID{Bytes: b, Valid: true},
		Name:  "test-key",
		Scope: "full_access",
	}
	return SetAPIKey(context.Background(), key)
}

func newOKHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestRateLimiter_UnderLimitAllows(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{RequestsPerMinute: 600, Burst: 5})
	defer rl.Stop()

	h := rl.Middleware()(newOKHandler())
	ctx := makeAPIKeyCtx(t, 0x11)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/api/v1/anything", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d (body=%s)", i, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-RateLimit-Limit"); got != "5" {
			t.Errorf("request %d: X-RateLimit-Limit = %q, want %q", i, got, "5")
		}
		if rec.Header().Get("X-RateLimit-Remaining") == "" {
			t.Errorf("request %d: missing X-RateLimit-Remaining", i)
		}
		if rec.Header().Get("X-RateLimit-Reset") == "" {
			t.Errorf("request %d: missing X-RateLimit-Reset", i)
		}
	}
}

func TestRateLimiter_OverLimitReturns429WithHeaders(t *testing.T) {
	// 60 rpm => 1 token/second refill, burst 3 — easy to exhaust.
	rl := NewRateLimiter(RateLimitConfig{RequestsPerMinute: 60, Burst: 3})
	defer rl.Stop()

	h := rl.Middleware()(newOKHandler())
	ctx := makeAPIKeyCtx(t, 0x22)

	// Drain the burst.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/api/v1/x", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("burst request %d expected 200, got %d", i, rec.Code)
		}
	}

	// 4th request must be rejected.
	req := httptest.NewRequest("GET", "/api/v1/x", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d (body=%s)", rec.Code, rec.Body.String())
	}

	if got := rec.Header().Get("X-RateLimit-Limit"); got != "3" {
		t.Errorf("X-RateLimit-Limit = %q, want %q", got, "3")
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "0" {
		t.Errorf("X-RateLimit-Remaining = %q, want %q", got, "0")
	}
	resetHdr := rec.Header().Get("X-RateLimit-Reset")
	if resetHdr == "" {
		t.Fatal("missing X-RateLimit-Reset")
	}
	resetTs, err := strconv.ParseInt(resetHdr, 10, 64)
	if err != nil {
		t.Fatalf("X-RateLimit-Reset not an integer: %v", err)
	}
	if resetTs < time.Now().Unix() {
		t.Errorf("X-RateLimit-Reset (%d) should be >= now (%d)", resetTs, time.Now().Unix())
	}

	retry := rec.Header().Get("Retry-After")
	if retry == "" {
		t.Fatal("missing Retry-After")
	}
	retrySec, err := strconv.Atoi(retry)
	if err != nil {
		t.Fatalf("Retry-After not an integer: %v", err)
	}
	if retrySec < 1 {
		t.Errorf("Retry-After = %d, want >= 1", retrySec)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error response: %v (body=%s)", err, rec.Body.String())
	}
	if resp.Error.Code != "RATE_LIMITED" {
		t.Errorf("error code = %q, want %q", resp.Error.Code, "RATE_LIMITED")
	}
	if resp.Error.Message == "" {
		t.Error("error message must not be empty")
	}
}

func TestRateLimiter_PerKeyIsolation(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{RequestsPerMinute: 60, Burst: 2})
	defer rl.Stop()

	h := rl.Middleware()(newOKHandler())
	ctxA := makeAPIKeyCtx(t, 0xAA)
	ctxB := makeAPIKeyCtx(t, 0xBB)

	// Drain key A.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/v1/x", nil).WithContext(ctxA)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("A burst %d: expected 200, got %d", i, rec.Code)
		}
	}
	// A should now be over.
	req := httptest.NewRequest("GET", "/api/v1/x", nil).WithContext(ctxA)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("A 3rd request: expected 429, got %d", rec.Code)
	}

	// B should still have a full burst.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/v1/x", nil).WithContext(ctxB)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("B request %d: expected 200, got %d (key isolation broken)", i, rec.Code)
		}
	}
}

func TestRateLimiter_IPFallbackForUnauthRequests(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{RequestsPerMinute: 60, Burst: 2})
	defer rl.Stop()

	h := rl.Middleware()(newOKHandler())

	send := func(remoteAddr string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/v1/x", nil)
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// Two requests from IP-1 should pass...
	for i := 0; i < 2; i++ {
		rec := send("10.0.0.1:1234")
		if rec.Code != http.StatusOK {
			t.Fatalf("IP-1 burst %d: expected 200, got %d", i, rec.Code)
		}
	}
	// ...third is throttled.
	if rec := send("10.0.0.1:5555"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("IP-1 over: expected 429, got %d", rec.Code)
	}

	// A different IP starts with a fresh bucket.
	rec := send("10.0.0.2:1234")
	if rec.Code != http.StatusOK {
		t.Fatalf("IP-2: expected 200 (fresh bucket), got %d", rec.Code)
	}
}

func TestRateLimiter_DefaultsApplied(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{}) // both zero → defaults
	defer rl.Stop()

	if rl.Limit() != DefaultRateLimitBurst {
		t.Errorf("Limit() = %d, want default %d", rl.Limit(), DefaultRateLimitBurst)
	}
	if rl.rpm != DefaultRateLimitRPM {
		t.Errorf("rpm = %d, want default %d", rl.rpm, DefaultRateLimitRPM)
	}
}

func TestRateLimiter_EvictsIdleBuckets(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{RequestsPerMinute: 60, Burst: 5})
	defer rl.Stop()

	h := rl.Middleware()(newOKHandler())
	ctx := makeAPIKeyCtx(t, 0xCC)

	req := httptest.NewRequest("GET", "/api/v1/x", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	rl.mu.Lock()
	if len(rl.buckets) != 1 {
		rl.mu.Unlock()
		t.Fatalf("expected 1 bucket, got %d", len(rl.buckets))
	}
	rl.mu.Unlock()

	// Force eviction: pretend it's well past the TTL.
	rl.evictIdle(time.Now().Add(2 * rateLimitIdleTTL))

	rl.mu.Lock()
	defer rl.mu.Unlock()
	if len(rl.buckets) != 0 {
		t.Fatalf("expected idle bucket evicted, got %d remaining", len(rl.buckets))
	}
}

func TestRateLimiter_StopIsIdempotent(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{})
	rl.Stop()
	rl.Stop() // must not panic / double-close.
}

// Smoke check that identifyRequester prefers the API key over the IP.
func TestIdentifyRequester_PrefersAPIKey(t *testing.T) {
	ctx := makeAPIKeyCtx(t, 0x77)
	req := httptest.NewRequest("GET", "/x", nil).WithContext(ctx)
	req.RemoteAddr = "1.2.3.4:5678"

	id, source := identifyRequester(req)
	if source != "key" {
		t.Errorf("source = %q, want %q", source, "key")
	}
	want := fmt.Sprintf("key:%s", "77777777-7777-7777-7777-777777777777")
	if id != want {
		t.Errorf("id = %q, want %q", id, want)
	}
}
