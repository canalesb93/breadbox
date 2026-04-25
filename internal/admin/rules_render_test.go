package admin

import (
	"context"
	"strings"
	"testing"

	"breadbox/internal/templates/components/pages"
)

// TestRulesTemplateRenders is the post-migration smoke test for the rules
// list page. The page now renders via templ (pages.Rules) instead of the
// removed pages/rules.html, so we render the component directly to a
// strings.Builder rather than going through TemplateRenderer.
func TestRulesTemplateRenders(t *testing.T) {
	props := pages.RulesProps{
		Rules:          []pages.RulesRow{},
		Total:          0,
		Page:           1,
		PageSize:       50,
		TotalPages:     1,
		ShowingStart:   0,
		ShowingEnd:     0,
		PaginationBase: "/rules?page=",
	}

	var buf strings.Builder
	if err := pages.Rules(props).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	html := buf.String()

	if !strings.Contains(html, `x-data="rulesPage"`) {
		t.Error("rules Alpine component not initialized")
	}
	if !strings.Contains(html, `src="/static/js/admin/components/rules.js"`) {
		t.Error("rules.js script tag not rendered")
	}
	if !strings.Contains(html, "No rules yet") {
		t.Error("empty-state message not rendered")
	}

	t.Logf("Rendered %d bytes", buf.Len())
}
