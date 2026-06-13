//go:build !headless && !lite

package pages

import (
	"strings"
	"testing"

	"breadbox/internal/service"
)

func strptr(s string) *string { return &s }

func TestBuildCategoryGroups(t *testing.T) {
	cats := []service.CategoryResponse{
		{
			ID:          "p1",
			Slug:        "food-dining",
			DisplayName: "Food & Dining",
			Color:       strptr("#22c55e"),
			Icon:        strptr("utensils"),
			Children: []service.CategoryResponse{
				{ID: "c1", Slug: "groceries", DisplayName: "Groceries", Color: strptr("#16a34a")},
				{ID: "c2", Slug: "restaurants", DisplayName: "Restaurants", Hidden: true},
			},
		},
		{
			ID:          "p2",
			Slug:        "income",
			DisplayName: "Income",
			IsSystem:    true,
			// No children — childless parent.
		},
	}

	groups := BuildCategoryGroups(cats)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}

	// Order preserved from the service layer.
	if groups[0].Parent.ID != "p1" || groups[1].Parent.ID != "p2" {
		t.Fatalf("group order not preserved: %q, %q", groups[0].Parent.ID, groups[1].Parent.ID)
	}

	// Parent VM: flattened color/icon, child count, not-a-child.
	p := groups[0].Parent
	if p.IsChild {
		t.Error("parent flagged as child")
	}
	if p.ChildCount != 2 {
		t.Errorf("ChildCount = %d, want 2", p.ChildCount)
	}
	if p.Color != "#22c55e" || p.Icon != "utensils" {
		t.Errorf("parent color/icon = %q/%q, want #22c55e/utensils", p.Color, p.Icon)
	}

	// Childless system parent.
	sys := groups[1].Parent
	if sys.ChildCount != 0 || !sys.IsSystem {
		t.Errorf("system parent: ChildCount=%d IsSystem=%v, want 0/true", sys.ChildCount, sys.IsSystem)
	}
	if len(groups[1].Children) != 0 {
		t.Errorf("childless parent has %d children, want 0", len(groups[1].Children))
	}

	// Children: flagged, carry parent name, colorless one resolves to "".
	kids := groups[0].Children
	if len(kids) != 2 {
		t.Fatalf("got %d children, want 2", len(kids))
	}
	if !kids[0].IsChild || kids[0].ParentName != "Food & Dining" {
		t.Errorf("child[0]: IsChild=%v ParentName=%q", kids[0].IsChild, kids[0].ParentName)
	}
	if kids[1].Color != "" {
		t.Errorf("colorless child Color = %q, want empty", kids[1].Color)
	}
	if !kids[1].Hidden {
		t.Error("hidden child not flagged hidden")
	}

	// Parent search index folds in every child so the header stays visible
	// when a filter matches a subcategory.
	for _, want := range []string{"food & dining", "food-dining", "groceries", "restaurants"} {
		if !strings.Contains(p.Search, want) {
			t.Errorf("parent Search missing %q: %q", want, p.Search)
		}
	}
	// Child search index folds in the parent name so a parent-name filter
	// keeps the children visible.
	if !strings.Contains(kids[0].Search, "groceries") || !strings.Contains(kids[0].Search, "food & dining") {
		t.Errorf("child Search missing own name or parent: %q", kids[0].Search)
	}
}

func TestCategoryRowDescriptor(t *testing.T) {
	tests := []struct {
		name string
		r    CategoryRowVM
		want string
	}{
		{"child shows slug", CategoryRowVM{IsChild: true, Slug: "groceries"}, "groceries"},
		{"parent none", CategoryRowVM{ChildCount: 0}, "No subcategories"},
		{"parent one", CategoryRowVM{ChildCount: 1}, "1 subcategory"},
		{"parent many", CategoryRowVM{ChildCount: 4}, "4 subcategories"},
	}
	for _, tc := range tests {
		if got := categoryRowDescriptor(tc.r); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCategoriesTopLevelLabel(t *testing.T) {
	if got := categoriesTopLevelLabel(1); got != "1 top-level category" {
		t.Errorf("singular: got %q", got)
	}
	if got := categoriesTopLevelLabel(3); got != "3 top-level categories" {
		t.Errorf("plural: got %q", got)
	}
}

func TestCategoryNameClass(t *testing.T) {
	// Parent reads heavier than a child.
	if cls := categoryNameClass(CategoryRowVM{}); !strings.Contains(cls, "font-semibold") {
		t.Errorf("parent class missing font-semibold: %q", cls)
	}
	if cls := categoryNameClass(CategoryRowVM{IsChild: true}); !strings.Contains(cls, "font-medium") {
		t.Errorf("child class missing font-medium: %q", cls)
	}
	// Hidden dims + italicizes.
	if cls := categoryNameClass(CategoryRowVM{Hidden: true}); !strings.Contains(cls, "italic") {
		t.Errorf("hidden class missing italic: %q", cls)
	}
}
