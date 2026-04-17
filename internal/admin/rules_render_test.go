package admin

import (
	"bytes"
	"strings"
	"testing"
)

func TestRulesTemplateRenders(t *testing.T) {
	tr, err := NewTemplateRenderer(nil)
	if err != nil {
		t.Fatalf("NewTemplateRenderer: %v", err)
	}

	data := map[string]interface{}{
		"PageTitle":      "Rules",
		"CurrentPage":    "rules",
		"CSRFToken":      "test",
		"Flash":          nil,
		"Rules":          []interface{}{},
		"HasMore":        false,
		"NextCursor":     "",
		"Total":          int64(0),
		"Page":           1,
		"PageSize":       50,
		"TotalPages":     1,
		"PaginationBase": "/rules?page=",
		"ShowingStart":   0,
		"ShowingEnd":     int64(0),
		"ActiveCount":    0,
		"DisabledCount":  0,
		"TotalHits":      0,
		"AgentCreated":   0,
		"SearchFilter":   "",
		"CategoryFilter": "",
		"EnabledFilter":  "",
		"FlatCategories": []interface{}{},
		"Version":        "dev",
	}

	var buf bytes.Buffer
	err = tr.RenderTo(&buf, "rules.html", data)
	if err != nil {
		t.Fatalf("RenderTo error: %v", err)
	}

	html := buf.String()

	if !strings.Contains(html, `rulesPage()`) {
		t.Error("rules Alpine component not initialized")
	}
	if !strings.Contains(html, "No rules yet") {
		t.Error("empty-state message not rendered")
	}

	t.Logf("Rendered %d bytes", buf.Len())
}
