//go:build !headless && !lite

package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// These tests cover the templ-adapter behavior — wrapping, variants, and
// that rendering delegates to internal/markdown. Deep parsing, GFM, syntax
// highlighting, and XSS sanitization are covered in internal/markdown.

func renderToString(t *testing.T, text string) string {
	t.Helper()
	return renderComponent(t, Markdown(text))
}

func renderComponent(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestMarkdown_Empty(t *testing.T) {
	if got := renderToString(t, ""); got != "" {
		t.Fatalf("expected empty render, got %q", got)
	}
	if got := renderToString(t, "   \n\n  "); got != "" {
		t.Fatalf("expected empty render for whitespace, got %q", got)
	}
}

func TestMarkdown_WrapsInProse(t *testing.T) {
	got := renderToString(t, "Hello **world**.")
	if !strings.HasPrefix(got, `<div class="bb-prose">`) || !strings.HasSuffix(got, "</div>") {
		t.Errorf("expected .bb-prose wrapper, got %q", got)
	}
	if !strings.Contains(got, "<strong>world</strong>") {
		t.Errorf("expected delegated emphasis, got %q", got)
	}
}

func TestMarkdownLarge_AddsSizeModifier(t *testing.T) {
	var buf bytes.Buffer
	if err := MarkdownLarge("Body text.").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.HasPrefix(buf.String(), `<div class="bb-prose bb-prose-lg">`) {
		t.Errorf("expected .bb-prose-lg modifier, got %q", buf.String())
	}
}

func TestMarkdownBreaks_HardWraps(t *testing.T) {
	var plain, breaks bytes.Buffer
	_ = Markdown("line one\nline two").Render(context.Background(), &plain)
	_ = MarkdownBreaks("line one\nline two").Render(context.Background(), &breaks)
	if strings.Contains(plain.String(), "<br") {
		t.Errorf("default Markdown should not hard-wrap, got %q", plain.String())
	}
	if !strings.Contains(breaks.String(), "<br") {
		t.Errorf("MarkdownBreaks should hard-wrap, got %q", breaks.String())
	}
}

func TestMarkdownInline_NoBlockWrapper(t *testing.T) {
	var buf bytes.Buffer
	if err := MarkdownInline("just **text**").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "<div") || strings.Contains(got, "<p>") {
		t.Errorf("inline render should have no block wrapper, got %q", got)
	}
	if !strings.Contains(got, "<strong>text</strong>") {
		t.Errorf("inline render should keep emphasis, got %q", got)
	}
}

func TestMarkdown_SanitizesViaPipeline(t *testing.T) {
	// Smoke check that the component output is sanitized (full coverage in
	// internal/markdown). A raw <script> must never survive.
	got := renderToString(t, "An <script>alert(1)</script> tag.")
	if strings.Contains(got, "<script") {
		t.Errorf("script not sanitized: %q", got)
	}
}
