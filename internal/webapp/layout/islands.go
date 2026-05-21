//go:build !headless && !lite

package layout

import "breadbox/internal/webapp/static/js/islands"

// IslandSrc resolves a logical island name (e.g. "palette") to its public URL under
// /app/static/js/islands/, using the content-hashed filename from the generated manifest
// (internal/webapp/cmd/buildjs writes it). If the manifest has no entry yet — e.g. before
// the first `make webapp-js` — it falls back to the unhashed "<name>.js" so templates still
// render a valid <script src>. Hashed bundles get immutable caching via embed.go.
func IslandSrc(name string) string {
	if file, ok := islands.Manifest[name]; ok {
		return "/app/static/js/islands/" + file
	}
	return "/app/static/js/islands/" + name + ".js"
}
