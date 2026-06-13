//go:build !lite

package api

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

// staticCacheControl is sent on every /static/* response. "no-cache" does NOT
// mean "don't cache" — it means the browser MAY cache the asset but MUST
// revalidate with the server before reusing it. Paired with a validator
// (ETag or Last-Modified) the server answers an unchanged asset with a cheap
// 304 Not Modified, and a changed asset (new deploy) with a fresh 200. This is
// what makes a cached admin JS/CSS bundle impossible to silently desync from
// newer server-rendered HTML — the regression class behind the /recurring
// "subscriptions.js missing method" bug, where a heuristically-cached bundle
// served stale against HTML that referenced a newer method.
//
// We deliberately avoid "no-store" (which kills caching outright and forces a
// full re-download on every navigation) and avoid a long max-age (which is the
// stale-cache trap itself, absent content-hashed filenames).
const staticCacheControl = "no-cache"

// NewStaticHandler builds the handler mounted at /static/*. It wraps the
// standard http.FileServer with two additions:
//
//  1. A Cache-Control: no-cache header on every response, forcing browsers to
//     revalidate cached assets instead of serving them heuristically-stale.
//
//  2. Content-hash ETags for the embedded FS. http.FileServer derives a
//     Last-Modified validator from the file's modtime and will answer
//     conditional requests with 304 — but embed.FS files all report a zero
//     modtime, so http.ServeContent emits no Last-Modified and cannot 304.
//     Without a validator, "no-cache" would still be correct (never stale) but
//     would re-transfer the full asset on every request. We precompute a
//     sha256-based ETag per embedded file at startup and set it on the
//     response so http.ServeContent can satisfy If-None-Match with a 304.
//
// In dev-reload mode (BREADBOX_DEV_RELOAD=1) files are read from disk and carry
// real, edit-updated modtimes, so http.FileServer's own Last-Modified handling
// already revalidates correctly. We skip the precomputed ETags there — they
// would go stale on the next edit and reintroduce the very bug we're fixing.
func NewStaticHandler(staticFS fs.FS, devReload bool) http.Handler {
	fileServer := http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))

	var etags map[string]string
	if !devReload {
		etags = buildETags(staticFS)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", staticCacheControl)
		if etags != nil {
			rel := strings.TrimPrefix(r.URL.Path, "/static/")
			if tag, ok := etags[rel]; ok {
				// http.ServeContent reads this header to honour If-None-Match
				// and emits the 304 itself; we just supply the validator.
				w.Header().Set("Etag", tag)
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}

// buildETags walks the embedded asset FS once and returns a map of
// relative-path → strong ETag (a quoted sha256 prefix of the file contents).
// Content-hashing means the validator changes exactly when the asset changes
// (i.e. on a new build/deploy), so stale 304s are impossible.
func buildETags(staticFS fs.FS) map[string]string {
	etags := make(map[string]string)
	_ = fs.WalkDir(staticFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		f, err := staticFS.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return nil
		}
		// 16 hex chars (64 bits) is ample to distinguish asset versions.
		etags[path] = `"` + hex.EncodeToString(h.Sum(nil))[:16] + `"`
		return nil
	})
	return etags
}
