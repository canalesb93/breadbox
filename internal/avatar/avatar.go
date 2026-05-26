// Package avatar fetches user identicons from the DiceBear HTTP API
// (https://www.dicebear.com) and processes uploaded image files.
//
// The current DiceBear style is stored in app_config under
// avatar.dicebear_style and pushed into the package via SetStyle at
// server startup and from the settings POST handler. GenerateSVG
// caches fetched SVGs in-process keyed by (style, seed, size) so a
// warm server serves identicons without round-tripping to DiceBear.
package avatar

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultStyle is used when no app_config value is set. "shapes"
	// keeps the previous abstract-geometric aesthetic.
	DefaultStyle = "shapes"

	// DefaultAPIBaseURL is the DiceBear v9 HTTP API root. Tests
	// override this via SetAPIBaseURL.
	DefaultAPIBaseURL = "https://api.dicebear.com/9.x"

	httpTimeout      = 5 * time.Second
	maxResponseBytes = 1 << 20 // 1 MiB — DiceBear SVGs are < 50 KiB

	// MaxSeedLength caps the seed string used both as a DB column
	// value and a DiceBear query parameter. Long seeds blow up the
	// upstream URL (servers reject >8 KiB) and the in-process cache
	// key. 128 chars is comfortably more than a UUID or any
	// reasonable human-readable identifier.
	MaxSeedLength = 128

	// maxCacheEntries caps the in-memory SVG cache. Reached only by
	// adversarial /avatars/preview/{seed} traffic — household-scale
	// real usage stays well under this. Once exceeded, ResetCache()
	// is called: cheap warm-up cost in exchange for a hard memory
	// bound. Each entry is ~3–50 KiB, so 1024 ≈ 50 MiB worst case.
	maxCacheEntries = 1024
)

var (
	styleAtomic  atomic.Value // string
	baseURL      atomic.Value // string
	defaultHTTPC = &http.Client{Timeout: httpTimeout}
	httpClient   atomic.Value // *http.Client — overridable in tests
	cache        sync.Map     // string -> []byte
	cacheCount   atomic.Int64 // entries currently in cache; bounded by maxCacheEntries
)

func init() {
	styleAtomic.Store(DefaultStyle)
	baseURL.Store(DefaultAPIBaseURL)
	httpClient.Store(defaultHTTPC)
}

// SetStyle overrides the DiceBear style at runtime. Call from server
// startup (with the value loaded from app_config) and from the
// settings POST handler when the operator changes the style.
func SetStyle(s string) {
	if s == "" {
		s = DefaultStyle
	}
	styleAtomic.Store(s)
}

// Style returns the currently configured DiceBear style.
func Style() string {
	v, _ := styleAtomic.Load().(string)
	if v == "" {
		return DefaultStyle
	}
	return v
}

// SetAPIBaseURL overrides the DiceBear API base URL. Test-only entry
// point so unit tests can point at an httptest.Server.
func SetAPIBaseURL(u string) {
	if u == "" {
		u = DefaultAPIBaseURL
	}
	baseURL.Store(u)
}

// SetHTTPClient overrides the HTTP client used to call DiceBear.
// Test-only; production uses the package default.
func SetHTTPClient(c *http.Client) {
	if c == nil {
		c = defaultHTTPC
	}
	httpClient.Store(c)
}

// ResetCache clears the in-memory SVG cache. Used by tests and by
// the cache-size guard when maxCacheEntries is exceeded.
func ResetCache() {
	cache.Range(func(k, _ any) bool {
		cache.Delete(k)
		return true
	})
	cacheCount.Store(0)
}

// GenerateSVG fetches a DiceBear avatar SVG for the given seed using
// the global style (or per-call override via GenerateSVGStyled).
// Cached in memory keyed by (style, size, seed); cache is hard-
// capped at maxCacheEntries so adversarial unauthenticated traffic
// to /avatars/preview/{seed} can't OOM the process. On upstream
// error, returns a small fallback SVG so the page still renders.
//
// The fallback path is NOT cached — a transient DiceBear outage
// would otherwise pin a broken circle in memory until the operator
// restarts.
func GenerateSVG(seed string, size int) []byte {
	return GenerateSVGStyled(seed, size, "")
}

// GenerateSVGStyled is GenerateSVG with a per-call style override.
// Used by the settings-page preview tiles so the operator can see
// each style before saving without flipping the global.
func GenerateSVGStyled(seed string, size int, styleOverride string) []byte {
	st := styleOverride
	if st == "" {
		st = Style()
	}
	key := cacheKey(st, size, seed)
	if v, ok := cache.Load(key); ok {
		return v.([]byte)
	}
	body, err := fetchFromDiceBear(st, seed, size)
	if err != nil {
		return fallbackSVG(seed, size)
	}
	if cacheCount.Add(1) > maxCacheEntries {
		ResetCache()
	}
	cache.Store(key, body)
	return body
}

// cacheKey returns a collision-free key for (style, size, seed). A
// hash avoids ambiguity from "/" appearing in attacker-supplied
// /avatars/preview/{seed} input — separator-based keys would let an
// attacker plant SVGs visible to other (style, size) lookups.
func cacheKey(style string, size int, seed string) string {
	h := sha256.Sum256([]byte(style + "\x00" + strconv.Itoa(size) + "\x00" + seed))
	return hex.EncodeToString(h[:])
}

func fetchFromDiceBear(style, seed string, size int) ([]byte, error) {
	base, _ := baseURL.Load().(string)
	if base == "" {
		base = DefaultAPIBaseURL
	}
	q := url.Values{}
	q.Set("seed", seed)
	if size > 0 {
		q.Set("size", strconv.Itoa(size))
	}
	endpoint := fmt.Sprintf("%s/%s/svg?%s", base, url.PathEscape(style), q.Encode())

	c, _ := httpClient.Load().(*http.Client)
	if c == nil {
		c = defaultHTTPC
	}
	resp, err := c.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dicebear: status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
}

// fallbackSVG produces a minimal, deterministic SVG when the DiceBear
// API is unreachable. Color is derived from the seed so the same user
// gets the same circle on every render until DiceBear comes back.
func fallbackSVG(seed string, size int) []byte {
	if size <= 0 {
		size = 256
	}
	h := sha256.Sum256([]byte(seed))
	hue := int(h[0]) * 360 / 255
	return []byte(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d"><circle cx="%d" cy="%d" r="%d" fill="hsl(%d 60%% 60%%)"/></svg>`,
		size, size, size, size, size/2, size/2, size/2, hue,
	))
}

// StyleOption is one entry in AvailableStyles. ID matches DiceBear's
// URL slug (https://api.dicebear.com/9.x/<id>/svg); Label is the
// human-readable name shown in the settings dropdown.
type StyleOption struct {
	ID    string
	Label string
}

// AvailableStyles is the catalog of DiceBear v9 styles surfaced in
// the settings UI. Ordered roughly from abstract (top) to character-
// based (bottom) so the selector reads naturally.
var AvailableStyles = []StyleOption{
	{ID: "shapes", Label: "Shapes (abstract)"},
	{ID: "rings", Label: "Rings"},
	{ID: "identicon", Label: "Identicon (GitHub-style)"},
	{ID: "glass", Label: "Glass"},
	{ID: "initials", Label: "Initials"},
	{ID: "thumbs", Label: "Thumbs"},
	{ID: "icons", Label: "Icons"},
	{ID: "bottts", Label: "Bottts (robots)"},
	{ID: "bottts-neutral", Label: "Bottts neutral"},
	{ID: "fun-emoji", Label: "Fun emoji"},
	{ID: "pixel-art", Label: "Pixel art"},
	{ID: "pixel-art-neutral", Label: "Pixel art neutral"},
	{ID: "adventurer", Label: "Adventurer"},
	{ID: "adventurer-neutral", Label: "Adventurer neutral"},
	{ID: "avataaars", Label: "Avataaars"},
	{ID: "avataaars-neutral", Label: "Avataaars neutral"},
	{ID: "big-ears", Label: "Big ears"},
	{ID: "big-ears-neutral", Label: "Big ears neutral"},
	{ID: "big-smile", Label: "Big smile"},
	{ID: "croodles", Label: "Croodles"},
	{ID: "croodles-neutral", Label: "Croodles neutral"},
	{ID: "dylan", Label: "Dylan"},
	{ID: "lorelei", Label: "Lorelei"},
	{ID: "lorelei-neutral", Label: "Lorelei neutral"},
	{ID: "micah", Label: "Micah"},
	{ID: "miniavs", Label: "Miniavs"},
	{ID: "notionists", Label: "Notionists"},
	{ID: "notionists-neutral", Label: "Notionists neutral"},
	{ID: "open-peeps", Label: "Open Peeps"},
	{ID: "personas", Label: "Personas"},
}

// IsValidStyle reports whether s appears in AvailableStyles. Used by
// the settings POST handler to reject arbitrary input before it
// reaches app_config.
func IsValidStyle(s string) bool {
	for _, opt := range AvailableStyles {
		if opt.ID == s {
			return true
		}
	}
	return false
}

// seedPattern restricts seeds to URL-safe characters that the
// DiceBear API and our cache key can both round-trip cleanly. UUIDs,
// short IDs, and the random hex strings the regenerate flow mints
// all satisfy it.
var seedPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// IsValidSeed reports whether s is safe to persist + send to
// DiceBear. Empty seeds are rejected; long seeds (>MaxSeedLength)
// are rejected to bound the upstream URL and cache key; characters
// outside [A-Za-z0-9._-] are rejected to keep the seed inert in
// URLs, logs, and the in-memory cache key.
func IsValidSeed(s string) bool {
	if s == "" || len(s) > MaxSeedLength {
		return false
	}
	return seedPattern.MatchString(s)
}
