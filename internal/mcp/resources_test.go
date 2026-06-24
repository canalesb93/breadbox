//go:build !lite

package mcp

import (
	"strings"
	"testing"
)

func TestBreadboxImplementation_IncludesMetadataAndIcon(t *testing.T) {
	impl := breadboxImplementation("1.2.3")
	if impl.Name != "breadbox" {
		t.Errorf("Name = %q, want breadbox", impl.Name)
	}
	if impl.Title != "Breadbox" {
		t.Errorf("Title = %q, want Breadbox", impl.Title)
	}
	if impl.Version != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3", impl.Version)
	}
	if impl.WebsiteURL != "https://breadbox.sh" {
		t.Errorf("WebsiteURL = %q, want https://breadbox.sh", impl.WebsiteURL)
	}
	if len(impl.Icons) != 1 {
		t.Fatalf("Icons = %d, want 1", len(impl.Icons))
	}
	icon := impl.Icons[0]
	if icon.MIMEType != "image/svg+xml" {
		t.Errorf("Icon MIMEType = %q, want image/svg+xml", icon.MIMEType)
	}
	if !strings.HasPrefix(icon.Source, "data:image/svg+xml;base64,") {
		t.Errorf("Icon Source should be a data URI, got %q", head(icon.Source, 40))
	}
	if len(icon.Sizes) != 1 || icon.Sizes[0] != "any" {
		t.Errorf("Icon Sizes = %v, want [\"any\"]", icon.Sizes)
	}
}

func head(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}
