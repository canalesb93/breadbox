//go:build !lite

package static

import (
	"embed"
	"io/fs"
	"os"
)

//go:embed all:css all:js all:fonts favicon.svg
var embedded embed.FS

// FS serves static assets. Defaults to the embedded FS baked into the binary.
// When BREADBOX_DEV_RELOAD=1 is set, it reads from disk so `make css-watch`
// output is picked up on every request without a rebuild. Source directory is
// BREADBOX_STATIC_DIR, or "static" if unset.
var FS fs.FS = embedded

// DevReload reports whether static assets are being served from disk
// (BREADBOX_DEV_RELOAD=1) rather than the embedded FS. The static handler uses
// this to decide its revalidation strategy: disk files carry real modtimes
// (Last-Modified validators that update on edit), whereas embedded files have a
// zero modtime and need content-hash ETags instead. See internal/api/static.go.
var DevReload = os.Getenv("BREADBOX_DEV_RELOAD") == "1"

func init() {
	if !DevReload {
		return
	}
	dir := os.Getenv("BREADBOX_STATIC_DIR")
	if dir == "" {
		dir = "static"
	}
	FS = os.DirFS(dir)
}
