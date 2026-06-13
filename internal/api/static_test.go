//go:build !lite

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// embeddedFS mimics the production embed.FS: files report a zero modtime, so
// http.ServeContent cannot derive a Last-Modified validator from them — the
// reason NewStaticHandler supplies content-hash ETags in embedded mode.
func embeddedFS() fstest.MapFS {
	return fstest.MapFS{
		"css/styles.css":            {Data: []byte("body{color:red}")},
		"js/admin/subscriptions.js": {Data: []byte("export function f(){}")},
		"favicon.svg":               {Data: []byte("<svg/>")},
	}
}

// Cache-Control: no-cache is the load-bearing header — it forces browsers to
// revalidate cached bundles instead of serving them heuristically-stale, which
// is the desync that broke /recurring (a cached subscriptions.js missing a
// method newer server HTML referenced).
func TestStaticHandler_SetsNoCache(t *testing.T) {
	h := NewStaticHandler(embeddedFS(), false)
	for _, path := range []string{"/static/css/styles.css", "/static/js/admin/subscriptions.js", "/static/favicon.svg"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", path, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control = %q, want %q", path, got, "no-cache")
		}
	}
}

// In embedded mode the handler must emit an ETag so that no-cache revalidation
// can return a cheap 304 (embed.FS files have no Last-Modified to validate
// against).
func TestStaticHandler_EmbeddedEmitsETagAnd304(t *testing.T) {
	h := NewStaticHandler(embeddedFS(), false)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/css/styles.css", nil))
	etag := rec.Header().Get("Etag")
	if etag == "" {
		t.Fatal("embedded mode: expected an ETag, got none")
	}

	// A conditional request carrying the same validator must 304.
	req := httptest.NewRequest(http.MethodGet, "/static/css/styles.css", nil)
	req.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusNotModified {
		t.Fatalf("conditional GET: status = %d, want 304", rec2.Code)
	}
	if got := rec2.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("304 response: Cache-Control = %q, want no-cache", got)
	}
}

// Different content must yield a different ETag so a new deploy never serves a
// stale 304.
func TestStaticHandler_ETagIsContentAddressed(t *testing.T) {
	tagFor := func(body string) string {
		fsys := fstest.MapFS{"css/styles.css": {Data: []byte(body)}}
		rec := httptest.NewRecorder()
		NewStaticHandler(fsys, false).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/css/styles.css", nil))
		return rec.Header().Get("Etag")
	}
	if tagFor("body{color:red}") == tagFor("body{color:blue}") {
		t.Error("ETag did not change when asset content changed")
	}
}

// In dev-reload mode we rely on http.FileServer's own (disk-modtime)
// Last-Modified validators, which update on every edit. The handler still sets
// no-cache but must NOT precompute its own ETags (they'd go stale on the next
// edit and reintroduce the bug).
func TestStaticHandler_DevReloadNoPrecomputedETag(t *testing.T) {
	h := NewStaticHandler(embeddedFS(), true)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/css/styles.css", nil))
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
	if got := rec.Header().Get("Etag"); got != "" {
		t.Errorf("dev-reload mode: unexpected precomputed ETag %q", got)
	}
}
