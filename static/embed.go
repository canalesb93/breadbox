package static

import (
	"embed"
	"io/fs"
	"os"
)

//go:embed all:css all:js favicon.svg
var embedded embed.FS

// FS serves static assets. Defaults to the embedded FS baked into the binary.
// When BREADBOX_DEV_RELOAD=1 is set, it reads from disk so `make css-watch`
// output is picked up on every request without a rebuild. Source directory is
// BREADBOX_STATIC_DIR, or "static" if unset.
var FS fs.FS = embedded

func init() {
	if os.Getenv("BREADBOX_DEV_RELOAD") != "1" {
		return
	}
	dir := os.Getenv("BREADBOX_STATIC_DIR")
	if dir == "" {
		dir = "static"
	}
	FS = os.DirFS(dir)
}
