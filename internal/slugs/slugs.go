// Package slugs contains small string helpers for working with slug
// identifiers used across Breadbox (tags, categories, and similar). It lives
// in a neutral leaf package so both service and sync can import it without
// creating a dependency cycle.
package slugs

import "strings"

// TitleCase converts a slug ("needs-review") to a title-cased display name
// ("Needs Review"). Separators recognized: '-', ':', '_'. Used as the default
// display_name when auto-creating tags from a slug (e.g., during sync when a
// rule's add_tag action references a slug that hasn't been registered yet).
func TitleCase(slug string) string {
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == ':' || r == '_'
	})
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}
