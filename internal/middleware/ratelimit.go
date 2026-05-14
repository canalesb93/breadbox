//go:build !lite

package middleware

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"breadbox/internal/pgconv"

	"golang.org/x/time/rate"
)

// Default rate-limit values used when env vars are unset or invalid.
const (
	DefaultRateLimitRPM   = 120
	DefaultRateLimitBurst = 60

	// rateLimitIdleTTL is how long a per-key bucket can sit unused before it
	// is evicted from memory by the sweeper. Keeps the in-memory map bounded
	// without recycling actively used keys.
	rateLimitIdleTTL = 10 * time.Minute

	// rateLimitSweepInterval controls how often the sweeper checks for
	// idle buckets. Half the TTL gives a reasonable upper bound on memory
	// pressure without burning CPU.
	rateLimitSweepInterval = 5 * time.Minute
)

// RateLimitConfig configures the per-key token bucket. Values are validated:
// RPM <= 0 falls back to DefaultRateLimitRPM, Burst <= 0 falls back to
// DefaultRateLimitBurst.
type RateLimitConfig struct {
	// RequestsPerMinute is the sustained token refill rate. Zero or negative
	// values mean "use the default".
	RequestsPerMinute int
	// Burst is the bucket capacity (max tokens at any moment).
	Burst int
}

// rateLimitBucket pairs a token bucket with the timestamp of its last access,
// used by the eviction sweeper.
type rateLimitBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter is a per-API-key token-bucket limiter with IP fallback for
// requests that lack an authenticated key (e.g. health probes that slip
// through). Buckets that go untouched for rateLimitIdleTTL are evicted by a
// background sweeper.
type RateLimiter struct {
	rpm   int
	burst int

	mu      sync.Mutex
	buckets map[string]*rateLimitBucket

	stopCh chan struct{}
	stopFn sync.Once
}

// NewRateLimiter returns a rate limiter ready to be wrapped via Middleware().
// Call Stop() to shut down the eviction sweeper goroutine when finished
// (typically only matters in tests; the limiter lives for the process
// lifetime in production).
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	rpm := cfg.RequestsPerMinute
	if rpm <= 0 {
		rpm = DefaultRateLimitRPM
	}
	burst := cfg.Burst
	if burst <= 0 {
		burst = DefaultRateLimitBurst
	}

	rl := &RateLimiter{
		rpm:     rpm,
		burst:   burst,
		buckets: make(map[string]*rateLimitBucket),
		stopCh:  make(chan struct{}),
	}
	go rl.sweep()
	return rl
}

// Stop terminates the background eviction sweeper. Safe to call multiple
// times. Primarily for tests; production callers can ignore it.
func (rl *RateLimiter) Stop() {
	rl.stopFn.Do(func() { close(rl.stopCh) })
}

// Limit returns the configured burst (the bucket capacity). This is the
// value reported as X-RateLimit-Limit on every response.
func (rl *RateLimiter) Limit() int {
	return rl.burst
}

// refillRate returns the per-second token refill rate.
func (rl *RateLimiter) refillRate() rate.Limit {
	return rate.Limit(float64(rl.rpm) / 60.0)
}

// getBucket returns the bucket for the given identifier, creating it on
// first use. Concurrent-safe.
func (rl *RateLimiter) getBucket(id string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	if b, ok := rl.buckets[id]; ok {
		b.lastSeen = now
		return b.limiter
	}
	limiter := rate.NewLimiter(rl.refillRate(), rl.burst)
	rl.buckets[id] = &rateLimitBucket{
		limiter:  limiter,
		lastSeen: now,
	}
	return limiter
}

// sweep evicts idle buckets on a fixed interval until Stop() is called.
func (rl *RateLimiter) sweep() {
	ticker := time.NewTicker(rateLimitSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.evictIdle(time.Now())
		}
	}
}

// evictIdle is exposed for tests; production callers go through sweep().
func (rl *RateLimiter) evictIdle(now time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for id, b := range rl.buckets {
		if now.Sub(b.lastSeen) > rateLimitIdleTTL {
			delete(rl.buckets, id)
		}
	}
}

// Middleware returns the http middleware enforcing the limit. The returned
// middleware is safe to use as a chi r.Use().
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, source := identifyRequester(r)
			limiter := rl.getBucket(id)
			res := limiter.Reserve()

			now := time.Now()
			delay := res.DelayFrom(now)

			// Compute remaining tokens after this request as max(0, floor(tokens())).
			// rate.Limiter.Tokens() reflects the post-reservation balance.
			remaining := int(math.Floor(limiter.Tokens()))
			if remaining < 0 {
				remaining = 0
			}

			// Reset is when the bucket fully refills to `burst`. We compute it
			// from the current token count so it's a stable wall-clock value
			// any caller can use, not a relative duration.
			tokensNeeded := float64(rl.burst) - limiter.Tokens()
			if tokensNeeded < 0 {
				tokensNeeded = 0
			}
			refillSeconds := 0.0
			if rate := float64(rl.refillRate()); rate > 0 {
				refillSeconds = tokensNeeded / rate
			}
			resetAt := now.Add(time.Duration(refillSeconds * float64(time.Second)))

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.burst))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

			if delay > 0 {
				// Caller has been throttled — return the reservation so it
				// doesn't permanently consume future capacity, then 429.
				res.Cancel()
				retryAfter := int(math.Ceil(delay.Seconds()))
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				_ = source // identifier source (key/ip) intentionally not surfaced to client
				WriteError(w, http.StatusTooManyRequests, "RATE_LIMITED",
					fmt.Sprintf("Rate limit exceeded; retry after %ds", retryAfter))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// identifyRequester returns a stable per-caller identifier and the source
// label ("key" or "ip"). Authenticated requests bucket by API key ID;
// otherwise we bucket by client IP so unauth probes still get rate-limited.
func identifyRequester(r *http.Request) (string, string) {
	if k := GetAPIKey(r.Context()); k != nil {
		if id := pgconv.FormatUUID(k.ID); id != "" {
			return "key:" + id, "key"
		}
	}
	return "ip:" + clientIP(r), "ip"
}

// clientIP extracts the best-guess client IP from the request. chi's
// middleware.RealIP runs upstream and rewrites RemoteAddr from
// X-Forwarded-For when present, so RemoteAddr is normally enough.
func clientIP(r *http.Request) string {
	// Strip optional :port from RemoteAddr.
	addr := r.RemoteAddr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "unknown"
	}
	return addr
}
