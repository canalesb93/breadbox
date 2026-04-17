package templates

import (
	"embed"
	"io/fs"
	"os"
)

//go:embed layout/*.html pages/*.html partials/*.html
var embedded embed.FS

// FS serves template files. Defaults to the embedded FS baked into the binary.
// When BREADBOX_DEV_RELOAD=1 is set, it reads from disk instead so edits apply
// without a rebuild. The source directory is BREADBOX_TEMPLATES_DIR, or
// "internal/templates" if unset (i.e. the repo root).
var FS fs.FS = embedded

// DevReload reports whether template reloading from disk is enabled.
var DevReload = os.Getenv("BREADBOX_DEV_RELOAD") == "1"

func init() {
	if !DevReload {
		return
	}
	dir := os.Getenv("BREADBOX_TEMPLATES_DIR")
	if dir == "" {
		dir = "internal/templates"
	}
	FS = os.DirFS(dir)
}
