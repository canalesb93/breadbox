package mcp

import (
	"context"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStaticMarkdownResource_Shape(t *testing.T) {
	cases := []struct {
		uri     string
		body    string
		mustHit string
	}{
		{"breadbox://rule-dsl", DefaultRuleDSL, "Transaction rule DSL"},
	}

	for _, tc := range cases {
		t.Run(tc.uri, func(t *testing.T) {
			handler := staticMarkdownResource(tc.uri, tc.body)
			result, err := handler(context.Background(), &mcpsdk.ReadResourceRequest{})
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if len(result.Contents) != 1 {
				t.Fatalf("expected 1 content block, got %d", len(result.Contents))
			}
			c := result.Contents[0]
			if c.URI != tc.uri {
				t.Errorf("URI mismatch: got %q want %q", c.URI, tc.uri)
			}
			if c.MIMEType != "text/markdown" {
				t.Errorf("MIMEType mismatch: got %q want text/markdown", c.MIMEType)
			}
			if !strings.Contains(c.Text, tc.mustHit) {
				t.Errorf("body missing expected marker %q (first 80 chars: %q)", tc.mustHit, head(c.Text, 80))
			}
		})
	}
}

func TestResourceAnnotations_FieldsPopulated(t *testing.T) {
	ann := resourceAnnotations(audienceUserAndAssistant, 0.8, staticPromptModTime)
	if len(ann.Audience) != 2 || ann.Audience[0] != "user" || ann.Audience[1] != "assistant" {
		t.Errorf("Audience not set as expected: %v", ann.Audience)
	}
	if ann.Priority != 0.8 {
		t.Errorf("Priority: got %v want 0.8", ann.Priority)
	}
	if ann.LastModified == "" {
		t.Error("LastModified should be populated when lastModifiedFn is non-nil")
	}

	annNoTime := resourceAnnotations(audienceAssistantOnly, 0.5, nil)
	if annNoTime.LastModified != "" {
		t.Errorf("LastModified should be empty when lastModifiedFn is nil, got %q", annNoTime.LastModified)
	}
}

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

func TestBuildServer_RegistersAllResources(t *testing.T) {
	// Resources don't execute at registration time — they only need a non-nil
	// MCPServer and BuildServer to wire them up successfully. The live
	// resources call svc.* methods only at read time, so nil-svc registration
	// is fine for this smoke check.
	s := NewMCPServer(nil, "test")
	server := s.BuildServer(MCPServerConfig{Mode: "read_write", APIKeyScope: "full_access"})
	if server == nil {
		t.Fatal("BuildServer returned nil")
	}
	// If we reached here without panicking, every AddResource call accepted
	// its handler signature. A deeper assertion (resources/list output) would
	// require an in-process MCP transport; the integration tests cover that.
}

func head(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}
