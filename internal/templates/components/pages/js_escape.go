//go:build !headless && !lite

package pages

import "strings"

// jsEscape is a minimal escape helper for embedding short tokens inside
// a single-quoted JS string literal. Not a general-purpose escaper — the
// CSRF token is base64url so it only needs ' and \ guarded.
func jsEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}
