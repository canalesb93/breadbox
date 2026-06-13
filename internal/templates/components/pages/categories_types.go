//go:build !headless && !lite

package pages

import (
	"fmt"
	"strings"

	"breadbox/internal/service"
)

// CategoriesProps mirrors the data the /categories page reads. The page is
// purely configuration-focused — no spending data or period selector — so
// only the category tree is needed. BuildCategoryGroups flattens it into the
// render model the templ consumes.
type CategoriesProps struct {
	Categories []service.CategoryResponse
}

// CategoryRowVM is the flattened view-model for one category list-row —
// parent or child. Pointer fields on service.CategoryResponse are
// dereferenced here so the templ never nil-checks per cell, and the
// per-row `data-search` index is precomputed so the Alpine filter matches
// with one includes() call.
type CategoryRowVM struct {
	ID          string
	Slug        string
	DisplayName string
	Color       string // resolved color (hex/oklch), "" when none set
	Icon        string // resolved lucide name, "" when none set
	IsSystem    bool
	Hidden      bool
	IsChild     bool
	ChildCount  int    // parents only — drives the count pill
	ParentName  string // children only — folded into Search + reads as context
	Search      string // lowercase data-search index for the Alpine filter
}

// CategoryGroup is one top-level category (the lead/parent row) plus its
// subcategory rows. One group renders as a single bb-card of list-rows: the
// parent as the emphasized first row, its children indented beneath.
type CategoryGroup struct {
	Parent   CategoryRowVM
	Children []CategoryRowVM
}

// BuildCategoryGroups flattens the service category tree into the page's
// render model — one group per top-level category, each carrying a parent
// row VM and its subcategory row VMs. Order is preserved from the service
// layer (DB sort_order). Pure, so the IA is unit-testable without a DB.
//
// The parent's Search index folds in every child so the parent header stays
// visible when a filter matches one of its subcategories; each child's index
// folds in the parent name so a parent-name filter keeps the children
// visible too.
func BuildCategoryGroups(cats []service.CategoryResponse) []CategoryGroup {
	groups := make([]CategoryGroup, 0, len(cats))
	for _, parent := range cats {
		pv := categoryRowVM(parent, false, "")
		pv.ChildCount = len(parent.Children)

		searchParts := []string{parent.DisplayName, parent.Slug}
		children := make([]CategoryRowVM, 0, len(parent.Children))
		for _, ch := range parent.Children {
			cv := categoryRowVM(ch, true, parent.DisplayName)
			cv.Search = strings.ToLower(strings.Join(
				[]string{ch.DisplayName, ch.Slug, parent.DisplayName}, " "))
			children = append(children, cv)
			searchParts = append(searchParts, ch.DisplayName, ch.Slug)
		}
		pv.Search = strings.ToLower(strings.Join(searchParts, " "))

		groups = append(groups, CategoryGroup{Parent: pv, Children: children})
	}
	return groups
}

// categoryRowVM builds the shared row view-model. Search is set by the
// caller (it needs sibling/parent context).
func categoryRowVM(c service.CategoryResponse, child bool, parentName string) CategoryRowVM {
	return CategoryRowVM{
		ID:          c.ID,
		Slug:        c.Slug,
		DisplayName: c.DisplayName,
		Color:       categoryDeref(c.Color),
		Icon:        categoryDeref(c.Icon),
		IsSystem:    c.IsSystem,
		Hidden:      c.Hidden,
		IsChild:     child,
		ParentName:  parentName,
	}
}

func categoryDeref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// categoriesTopLevelLabel renders the dimmed "N top-level categories" helper
// line above the groups.
func categoriesTopLevelLabel(n int) string {
	if n == 1 {
		return "1 top-level category"
	}
	return fmt.Sprintf("%d top-level categories", n)
}

// categoryRowDescriptor returns the muted one-line descriptor under a row's
// name: the subcategory count for a parent (the entity's shape at a glance),
// the slug for a child (its stable handle, the value rules and
// /transactions?category= reference).
func categoryRowDescriptor(r CategoryRowVM) string {
	if r.IsChild {
		return r.Slug
	}
	switch r.ChildCount {
	case 0:
		return "No subcategories"
	case 1:
		return "1 subcategory"
	default:
		return fmt.Sprintf("%d subcategories", r.ChildCount)
	}
}
