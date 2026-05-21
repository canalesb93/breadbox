//go:build !headless && !lite

package layout

// assetFallback maps a logical top-level asset name to its unhashed public URL.
// AssetURL uses it when the generated manifest (assets_manifest.go) has no entry
// — e.g. before the first `make webapp-assets`, or in dev where un-fingerprinted
// is fine — so templates always render a valid href/src.
var assetFallback = map[string]string{
	"app.css": "/app/static/css/app.css",
	"app.js":  "/app/static/js/app.js",
}

// AssetURL resolves a logical top-level asset name (e.g. "app.css") to its public
// URL under /app/static/, preferring the content-hashed filename from the generated
// manifest (internal/webapp/cmd/fingerprint writes it). When the manifest has no
// entry it falls back to the unhashed path, so dev and un-fingerprinted builds still
// serve a working asset. Hashed copies get immutable caching via embed.go.
func AssetURL(name string) string {
	if url, ok := Assets[name]; ok {
		return url
	}
	return assetFallback[name]
}
