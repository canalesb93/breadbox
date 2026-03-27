package admin

import (
	"net/http"
	"testing"
)

func TestBuildPaginationBase_URLEncodesValues(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		contains string // substring that must appear in result
		excludes string // substring that must NOT appear in result
	}{
		{
			name:     "ampersand in search",
			query:    "search=foo%26bar",
			contains: "search=foo%26bar",
			excludes: "search=foo&bar&",
		},
		{
			name:     "equals sign in search",
			query:    "search=foo%3Dbar",
			contains: "search=foo%3Dbar",
		},
		{
			name:     "plus sign in search",
			query:    "search=foo%2Bbar",
			contains: "search=foo%2Bbar",
		},
		{
			name:     "space in search",
			query:    "search=foo+bar",
			contains: "search=foo+bar",
		},
		{
			name:     "normal value passes through",
			query:    "search=groceries&pending=true",
			contains: "search=groceries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/transactions?"+tt.query, nil)
			result := buildPaginationBase(req)

			if tt.contains != "" {
				if !containsSubstring(result, tt.contains) {
					t.Errorf("expected result to contain %q, got %q", tt.contains, result)
				}
			}
			if tt.excludes != "" {
				if containsSubstring(result, tt.excludes) {
					t.Errorf("expected result NOT to contain %q, got %q", tt.excludes, result)
				}
			}
		})
	}
}

func TestBuildExportURL_URLEncodesValues(t *testing.T) {
	req, _ := http.NewRequest("GET", "/transactions?search=foo%26bar", nil)
	result := buildExportURL(req)

	if !containsSubstring(result, "search=foo%26bar") {
		t.Errorf("expected URL-encoded ampersand in export URL, got %q", result)
	}
	// The raw ampersand should not appear as a parameter separator within the value
	if containsSubstring(result, "search=foo&bar") {
		t.Errorf("raw ampersand should be encoded in export URL, got %q", result)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
