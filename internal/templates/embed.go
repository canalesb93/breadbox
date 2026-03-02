package templates

import "embed"

//go:embed layout/*.html pages/*.html partials/*.html
var FS embed.FS
