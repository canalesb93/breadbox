// Package strutil provides small, dependency-free helpers for string values.
// Lives alongside sliceutil and pgconv as the home for primitive helpers
// shared across admin, service, and provider layers.
package strutil

// Deref returns the dereferenced string, or "" when p is nil.
func Deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// DerefOr returns the dereferenced string, or def when p is nil.
func DerefOr(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}
