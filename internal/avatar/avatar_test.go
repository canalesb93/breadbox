package avatar

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// withDiceBearMock spins up an httptest server, points the package at
// it, and resets the cache. The returned cleanup restores previous
// package state and tears the server down.
func withDiceBearMock(t *testing.T, handler http.HandlerFunc) func() {
	t.Helper()
	srv := httptest.NewServer(handler)
	prevBase, _ := baseURL.Load().(string)
	prevStyle, _ := styleAtomic.Load().(string)
	SetAPIBaseURL(srv.URL)
	ResetCache()
	return func() {
		srv.Close()
		SetAPIBaseURL(prevBase)
		SetStyle(prevStyle)
		ResetCache()
	}
}

func TestGenerateSVG_FetchesFromDiceBear(t *testing.T) {
	cleanup := withDiceBearMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg data-style="` + r.URL.Path + `" data-seed="` + r.URL.Query().Get("seed") + `"/>`))
	})
	defer cleanup()

	SetStyle("identicon")

	svg := string(GenerateSVG("alice", 256))
	if !strings.Contains(svg, "/identicon/svg") {
		t.Errorf("expected style path in URL, got %q", svg)
	}
	if !strings.Contains(svg, `data-seed="alice"`) {
		t.Errorf("expected seed echoed back, got %q", svg)
	}
}

func TestGenerateSVG_CachesPerSeed(t *testing.T) {
	var hits int32
	cleanup := withDiceBearMock(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`<svg/>`))
	})
	defer cleanup()

	_ = GenerateSVG("alice", 256)
	_ = GenerateSVG("alice", 256)
	_ = GenerateSVG("alice", 256)

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("expected 1 upstream hit for repeated same-seed calls, got %d", got)
	}

	_ = GenerateSVG("bob", 256)
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("expected 2 upstream hits after new seed, got %d", got)
	}
}

func TestGenerateSVG_StyleChangeBustsCache(t *testing.T) {
	var hits int32
	cleanup := withDiceBearMock(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`<svg style="` + r.URL.Path + `"/>`))
	})
	defer cleanup()

	SetStyle("shapes")
	_ = GenerateSVG("alice", 256)
	SetStyle("bottts")
	_ = GenerateSVG("alice", 256)

	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("expected 2 upstream hits when style changes, got %d", got)
	}
}

func TestGenerateSVG_FallbackOnUpstreamError(t *testing.T) {
	cleanup := withDiceBearMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer cleanup()

	svg := string(GenerateSVG("alice", 256))
	if !strings.HasPrefix(svg, "<svg") || !strings.Contains(svg, "<circle") {
		t.Errorf("expected fallback circle SVG, got %q", svg)
	}
}

func TestGenerateSVG_FallbackOnNetworkError(t *testing.T) {
	prevBase, _ := baseURL.Load().(string)
	SetAPIBaseURL("http://127.0.0.1:1") // closed port — connect refused
	ResetCache()
	defer func() {
		SetAPIBaseURL(prevBase)
		ResetCache()
	}()

	svg := string(GenerateSVG("alice", 256))
	if !strings.HasPrefix(svg, "<svg") {
		t.Errorf("expected fallback SVG on network error, got %q", svg)
	}
}

func TestFallbackSVG_Deterministic(t *testing.T) {
	a := fallbackSVG("seed-1", 256)
	b := fallbackSVG("seed-1", 256)
	if string(a) != string(b) {
		t.Errorf("fallback SVG not deterministic for same seed")
	}
	c := fallbackSVG("seed-2", 256)
	if string(a) == string(c) {
		t.Errorf("fallback SVG identical for different seeds — color should vary")
	}
}

func TestIsValidStyle(t *testing.T) {
	if !IsValidStyle("shapes") {
		t.Error("shapes should be a valid style")
	}
	if IsValidStyle("not-a-real-style") {
		t.Error("arbitrary string should not be a valid style")
	}
	if IsValidStyle("") {
		t.Error("empty string should not be a valid style")
	}
}

func TestIsValidSeed(t *testing.T) {
	tests := []struct {
		name string
		seed string
		want bool
	}{
		{"empty rejected", "", false},
		{"uuid accepted", "8d331c40-28af-4b49-99e8-042c5231b849", true},
		{"hex random accepted", "abc123def456", true},
		{"alphanumeric+dash+underscore+dot", "user_2.alt-1", true},
		{"slash rejected", "shapes/256/bob", false},
		{"space rejected", "alice bob", false},
		{"control char rejected", "alice\nbob", false},
		{"unicode rejected", "alice_éü", false},
		{"too long rejected", strings.Repeat("a", MaxSeedLength+1), false},
		{"max length accepted", strings.Repeat("a", MaxSeedLength), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidSeed(tt.seed); got != tt.want {
				t.Errorf("IsValidSeed(%q) = %v, want %v", tt.seed, got, tt.want)
			}
		})
	}
}

func TestCacheKey_NoSeparatorCollisions(t *testing.T) {
	// Confirms the hash-based key avoids the old `style + "/" + size +
	// "/" + seed` collision where seed=`8/bob` under (shapes, 256)
	// would key-collide with seed=`bob` under (shapes, 256) at size=8.
	a := cacheKey("shapes", 256, "8/bob")
	b := cacheKey("shapes", 8, "bob")
	if a == b {
		t.Errorf("expected distinct keys, both = %q", a)
	}
}

func TestGenerateSVG_CacheCapped(t *testing.T) {
	// Once cacheCount exceeds maxCacheEntries, the next Store triggers
	// ResetCache(). Without the cap, a flood of unique seeds would
	// grow the sync.Map without bound.
	var hits int32
	cleanup := withDiceBearMock(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`<svg/>`))
	})
	defer cleanup()

	for i := 0; i < maxCacheEntries+10; i++ {
		_ = GenerateSVG("seed-"+strconv.Itoa(i), 256)
	}
	got := cacheCount.Load()
	if got > int64(maxCacheEntries) {
		t.Errorf("cacheCount = %d, want <= %d", got, maxCacheEntries)
	}
}

func TestGenerateSVGStyled_OverridesGlobal(t *testing.T) {
	cleanup := withDiceBearMock(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<svg path="` + r.URL.Path + `"/>`))
	})
	defer cleanup()

	SetStyle("shapes")
	got := string(GenerateSVGStyled("alice", 256, "identicon"))
	if !strings.Contains(got, "/identicon/svg") {
		t.Errorf("override ignored — expected identicon path in %q", got)
	}
	// Global wasn't touched.
	if Style() != "shapes" {
		t.Errorf("Style() = %q after override, want shapes", Style())
	}
}

func TestSetStyle_DefaultOnEmpty(t *testing.T) {
	prev, _ := styleAtomic.Load().(string)
	defer SetStyle(prev)

	SetStyle("")
	if got := Style(); got != DefaultStyle {
		t.Errorf("Style() = %q after SetStyle(\"\"); want %q", got, DefaultStyle)
	}
}

func TestProcessUpload_UnsupportedType(t *testing.T) {
	_, _, err := ProcessUpload([]byte("not an image"), "image/webp")
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}

func TestProcessUpload_CorruptImage(t *testing.T) {
	_, _, err := ProcessUpload([]byte("corrupt data"), "image/png")
	if err == nil {
		t.Error("expected error for corrupt image data")
	}
}

func TestProcessUpload_Success(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		encode      func(*bytes.Buffer, image.Image) error
		width       int
		height      int
	}{
		{
			name:        "square png",
			contentType: "image/png",
			encode:      func(b *bytes.Buffer, img image.Image) error { return png.Encode(b, img) },
			width:       300,
			height:      300,
		},
		{
			name:        "landscape jpeg",
			contentType: "image/jpeg",
			encode:      func(b *bytes.Buffer, img image.Image) error { return jpeg.Encode(b, img, nil) },
			width:       400,
			height:      200,
		},
		{
			name:        "portrait gif",
			contentType: "image/gif",
			encode:      func(b *bytes.Buffer, img image.Image) error { return gif.Encode(b, img, nil) },
			width:       200,
			height:      400,
		},
		{
			name:        "small upscale png",
			contentType: "image/png",
			encode:      func(b *bytes.Buffer, img image.Image) error { return png.Encode(b, img) },
			width:       64,
			height:      64,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := makeSolidImage(tt.width, tt.height, color.RGBA{R: 200, G: 100, B: 50, A: 255})
			var buf bytes.Buffer
			if err := tt.encode(&buf, src); err != nil {
				t.Fatalf("encode source: %v", err)
			}

			out, outType, err := ProcessUpload(buf.Bytes(), tt.contentType)
			if err != nil {
				t.Fatalf("ProcessUpload: %v", err)
			}
			if outType != "image/png" {
				t.Errorf("output content-type = %q, want image/png", outType)
			}
			decoded, err := png.Decode(bytes.NewReader(out))
			if err != nil {
				t.Fatalf("decode output: %v", err)
			}
			b := decoded.Bounds()
			if b.Dx() != targetSize || b.Dy() != targetSize {
				t.Errorf("output size = %dx%d, want %dx%d", b.Dx(), b.Dy(), targetSize, targetSize)
			}
		})
	}
}

func TestCenterCrop(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		wantSize      int
	}{
		{"already square", 100, 100, 100},
		{"landscape crops to height", 300, 150, 150},
		{"portrait crops to width", 150, 300, 150},
		{"tall strip", 50, 500, 50},
		{"wide strip", 500, 50, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := makeSolidImage(tt.width, tt.height, color.RGBA{A: 255})
			got := centerCrop(src)
			b := got.Bounds()
			if b.Dx() != tt.wantSize || b.Dy() != tt.wantSize {
				t.Errorf("cropped size = %dx%d, want %dx%d", b.Dx(), b.Dy(), tt.wantSize, tt.wantSize)
			}
		})
	}
}

func TestCenterCrop_FallbackNonSubImager(t *testing.T) {
	src := nonSubImager{makeSolidImage(200, 100, color.RGBA{R: 255, A: 255})}
	got := centerCrop(src)
	b := got.Bounds()
	if b.Dx() != 100 || b.Dy() != 100 {
		t.Errorf("fallback cropped size = %dx%d, want 100x100", b.Dx(), b.Dy())
	}
}

func makeSolidImage(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

type nonSubImager struct {
	image.Image
}
