package pages

import (
	"strings"

	"breadbox/internal/service"
)

// CategoriesProps mirrors the data map the old categories.html read. The
// page is purely configuration-focused — no spending data or period
// selector — so only the category tree is needed.
type CategoriesProps struct {
	Categories []service.CategoryResponse
}

// categorySearchIndex returns a single space-joined lowercase string
// containing the parent's display name / slug plus every child's display
// name / slug. Rendered into a `data-search` attribute so the Alpine
// filter input can match on any term with one includes() call.
func categorySearchIndex(c service.CategoryResponse) string {
	parts := make([]string, 0, 2+len(c.Children)*2)
	parts = append(parts, c.DisplayName, c.Slug)
	for _, ch := range c.Children {
		parts = append(parts, ch.DisplayName, ch.Slug)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// categoryChildSearchIndex is the child-row analogue: just the child's own
// display name and slug, lowercased for direct substring matching.
func categoryChildSearchIndex(c service.CategoryResponse) string {
	return strings.ToLower(c.DisplayName + " " + c.Slug)
}
