//go:build !lite

package service

import "testing"

func TestMCPClientFingerprint(t *testing.T) {
	cases := []struct {
		name      string
		client    MCPClientInfo
		transport string
		want      string
	}{
		{
			name:      "title wins over name",
			client:    MCPClientInfo{Name: "claude", Title: "Claude Desktop", WebsiteURL: "https://claude.ai"},
			transport: "stdio",
			want:      "claude_desktop@claude.ai@stdio",
		},
		{
			name:      "name fallback when no title",
			client:    MCPClientInfo{Name: "cursor", WebsiteURL: "https://cursor.sh"},
			transport: "stdio",
			want:      "cursor@cursor.sh@stdio",
		},
		{
			name:      "no website is empty host",
			client:    MCPClientInfo{Name: "myscript"},
			transport: "stdio",
			want:      "myscript@@stdio",
		},
		{
			name:      "uppercase + whitespace normalisation",
			client:    MCPClientInfo{Title: "  Claude  Desktop  ", WebsiteURL: "HTTPS://CLAUDE.AI/x"},
			transport: "http",
			want:      "claude_desktop@claude.ai@http",
		},
		{
			name:      "completely empty falls back to reserved singleton",
			client:    MCPClientInfo{},
			transport: "stdio",
			want:      MCPClientFallbackFingerprint,
		},
		{
			name:      "title-only with no website",
			client:    MCPClientInfo{Title: "VS Code"},
			transport: "stdio",
			want:      "vs_code@@stdio",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MCPClientFingerprint(tc.client, tc.transport)
			if got != tc.want {
				t.Fatalf("MCPClientFingerprint = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCollapseWhitespaceTrim covers the trailing-underscore strip that
// keeps fingerprints with trailing whitespace from collecting an
// orphan `_` separator.
func TestCollapseWhitespaceTrim(t *testing.T) {
	cases := map[string]string{
		"foo bar":      "foo_bar",
		"  foo  bar  ": "foo_bar",
		"foo":          "foo",
		"  foo":        "foo",
		"foo  ":        "foo",
		"":             "",
	}
	for in, want := range cases {
		if got := collapseWhitespace(in); got != want {
			t.Errorf("collapseWhitespace(%q) = %q, want %q", in, got, want)
		}
	}
}
