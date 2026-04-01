package admin

import (
	"bytes"
	"strings"
	"testing"

	"breadbox/internal/service"
)

func TestRulesTemplateWithCategories(t *testing.T) {
	tr, err := NewTemplateRenderer(nil)
	if err != nil {
		t.Fatalf("NewTemplateRenderer: %v", err)
	}

	type catSpending struct {
		Amount           float64
		TransactionCount int64
		Percent          float64
	}

	color := "#ef4444"
	icon := "utensils"

	data := map[string]interface{}{
		"PageTitle":      "Rules & Categories",
		"CurrentPage":    "rules",
		"CSRFToken":      "test",
		"Flash":          nil,
		"Tab":            "categories",
		"Rules":          []interface{}{},
		"HasMore":        false,
		"NextCursor":     "",
		"Total":          int64(0),
		"ActiveCount":    0,
		"DisabledCount":  0,
		"TotalHits":      0,
		"AgentCreated":   0,
		"SearchFilter":   "",
		"CategoryFilter": "",
		"EnabledFilter":  "",
		"Categories": []service.CategoryResponse{
			{
				ID: "1", DisplayName: "Food & Drink", Slug: "food_and_drink",
				Color: &color, Icon: &icon,
				Children: []service.CategoryResponse{
					{ID: "2", DisplayName: "Groceries", Slug: "food_and_drink_groceries"},
				},
			},
		},
		"FlatCategories":     []interface{}{},
		"Version":            "dev",
		"SpendingByCategory": map[string]catSpending{"Food & Drink": {Amount: 500.0, TransactionCount: 10, Percent: 50.0}},
		"TotalSpending":      1000.0,
		"MaxCategorySpend":   500.0,
		"SpendingDays":       30,
	}

	var buf bytes.Buffer
	err = tr.RenderTo(&buf, "rules.html", data)
	if err != nil {
		t.Fatalf("RenderTo error: %v", err)
	}

	html := buf.String()

	// Verify the categories tab content is rendered
	if !strings.Contains(html, "Food &amp; Drink") {
		t.Error("category 'Food & Drink' not found in rendered output")
	}
	if !strings.Contains(html, "Groceries") {
		t.Error("subcategory 'Groceries' not found in rendered output")
	}

	// Verify both tab x-show directives exist
	if !strings.Contains(html, `x-show="rcTab === 'rules'"`) {
		t.Error("rules tab x-show directive not found")
	}
	if !strings.Contains(html, `x-show="rcTab === 'categories'"`) {
		t.Error("categories tab x-show directive not found")
	}

	// Verify x-show and x-data are on SEPARATE elements (not same div)
	if strings.Contains(html, `x-show="rcTab === 'rules'" x-data="rulesPage()"`) {
		t.Error("x-show and x-data should be on separate elements, not combined")
	}

	// Verify the Alpine tab state initializes correctly
	if !strings.Contains(html, `x-data="{ rcTab: 'categories' }"`) {
		t.Error("Alpine tab state should initialize to 'categories' for this test")
	}

	// Verify create-cat-modal exists for the categories tab
	if !strings.Contains(html, `id="create-cat-modal"`) {
		t.Error("create-cat-modal dialog not found")
	}

	t.Logf("Rendered %d bytes", buf.Len())
}
