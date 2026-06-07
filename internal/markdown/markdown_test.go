//go:build !headless && !lite

package markdown

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		contains []string // substrings that MUST appear
		absent   []string // substrings that must NOT appear
	}{
		{
			name:     "inline emphasis and code",
			in:       "This is **bold**, *italic*, and `code`.",
			contains: []string{"<strong>bold</strong>", "<em>italic</em>", "<code>code</code>"},
		},
		{
			name:     "headings render at source level (no shift)",
			in:       "# Top\n\n## Sub",
			contains: []string{"<h1", "Top", "<h2", "Sub"},
		},
		{
			name:     "heading gets auto id",
			in:       "## Hello World",
			contains: []string{`id="hello-world"`},
		},
		{
			name:     "gfm table",
			in:       "| A | B |\n|:--|--:|\n| 1 | 2 |",
			contains: []string{"<table>", "<th", "<td", `align="left"`, `align="right"`},
		},
		{
			name:     "task list checkboxes",
			in:       "- [x] done\n- [ ] todo",
			contains: []string{`type="checkbox"`, "checked", "disabled"},
		},
		{
			name:     "strikethrough",
			in:       "~~gone~~",
			contains: []string{"<del>gone</del>"},
		},
		{
			name:     "fenced code is highlighted with chroma classes",
			in:       "```go\nfunc main() {}\n```",
			contains: []string{"chroma", `class="kd"`, "main"},
		},
		{
			name:     "typographer smart punctuation",
			in:       `He said "hi" -- really...`,
			contains: []string{"“hi”", "–", "…"}, // curly quotes, en dash, ellipsis
		},
		{
			name:     "bare url autolinks",
			in:       "see https://example.com now",
			contains: []string{`href="https://example.com"`},
		},
		{
			name:     "ordered and unordered lists",
			in:       "- one\n- two\n\n1. first\n2. second",
			contains: []string{"<ul>", "<ol>", "<li>one</li>"},
		},
		{
			name:     "blockquote",
			in:       "> quoted",
			contains: []string{"<blockquote>", "quoted"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := string(Render(tc.in))
			for _, want := range tc.contains {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\n--- got ---\n%s", want, out)
				}
			}
			for _, bad := range tc.absent {
				if strings.Contains(out, bad) {
					t.Errorf("output unexpectedly contains %q\n--- got ---\n%s", bad, out)
				}
			}
		})
	}
}

func TestRenderSanitizesXSS(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		absent []string
	}{
		{
			name:   "script tag stripped",
			in:     "hello <script>alert('xss')</script> world",
			absent: []string{"<script", "alert('xss')"},
		},
		{
			name:   "javascript link scheme dropped",
			in:     "[click](javascript:alert(1))",
			absent: []string{"javascript:", "alert(1)"},
		},
		{
			name:   "raw img onerror stripped",
			in:     "before <img src=x onerror=alert(1)> after",
			absent: []string{"onerror", "<img"},
		},
		{
			name:   "raw html event handler dropped",
			in:     "<a href=\"#\" onclick=\"steal()\">x</a>",
			absent: []string{"onclick", "steal()"},
		},
		{
			name:   "data uri dropped",
			in:     "[x](data:text/html,<script>alert(1)</script>)",
			absent: []string{"data:text/html", "<script"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := string(Render(tc.in))
			for _, bad := range tc.absent {
				if strings.Contains(out, bad) {
					t.Errorf("XSS not neutralized, found %q\n--- got ---\n%s", bad, out)
				}
			}
		})
	}
}

func TestHardWraps(t *testing.T) {
	in := "line one\nline two"
	if got := string(Render(in)); strings.Contains(got, "<br") {
		t.Errorf("default render should not hard-wrap, got %q", got)
	}
	if got := string(Render(in, WithHardWraps())); !strings.Contains(got, "<br") {
		t.Errorf("WithHardWraps should emit <br>, got %q", got)
	}
}

func TestRenderInlineUnwrapsParagraph(t *testing.T) {
	out := string(RenderInline("just **text**"))
	if strings.Contains(out, "<p>") || strings.Contains(out, "</p>") {
		t.Errorf("inline render should not contain <p>, got %q", out)
	}
	if !strings.Contains(out, "<strong>text</strong>") {
		t.Errorf("inline render should keep emphasis, got %q", out)
	}
}

func TestRenderEmpty(t *testing.T) {
	if got := Render("   "); got != "" {
		t.Errorf("blank input should render empty, got %q", got)
	}
}

func TestCallouts(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		contains []string
	}{
		{
			name:     "note callout",
			in:       "> [!NOTE]\n> Heads up on this charge.",
			contains: []string{`class="bb-callout bb-callout-note"`, "bb-callout-title", "<span>Note</span>", "Heads up on this charge."},
		},
		{
			name:     "warning callout case-insensitive",
			in:       "> [!warning]\n> Duplicate detected.",
			contains: []string{"bb-callout-warning", "<span>Warning</span>", "Duplicate detected."},
		},
		{
			name:     "marker stripped from body",
			in:       "> [!TIP]\n> Use a rule for this.",
			contains: []string{"bb-callout-tip", "Use a rule for this."},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := string(Render(tc.in))
			if strings.Contains(out, "[!") {
				t.Errorf("callout marker leaked into output: %s", out)
			}
			for _, want := range tc.contains {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q\n--- got ---\n%s", want, out)
				}
			}
		})
	}
}

func TestPlainBlockquoteStillRenders(t *testing.T) {
	out := string(Render("> just a quote"))
	if !strings.Contains(out, "<blockquote>") || strings.Contains(out, "bb-callout") {
		t.Errorf("plain blockquote should not be a callout: %s", out)
	}
}

func TestHeadingAnchor(t *testing.T) {
	out := string(Render("## Spending Review"))
	if !strings.Contains(out, `id="spending-review"`) {
		t.Errorf("missing heading id: %s", out)
	}
	if !strings.Contains(out, `class="bb-heading-anchor" href="#spending-review"`) {
		t.Errorf("missing hover-anchor link: %s", out)
	}
}

func TestCodeBlockChrome(t *testing.T) {
	out := string(Render("```go\nfunc main() {}\n```"))
	for _, want := range []string{`class="bb-code"`, "bb-code-bar", "bb-code-copy", `data-bb-copy`, `class="bb-code-lang">go<`} {
		if !strings.Contains(out, want) {
			t.Errorf("code chrome missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestCodeBlockChromeNoLanguage(t *testing.T) {
	// A fenced block with no language still gets the chrome + a <pre><code>.
	out := string(Render("```\nplain text\n```"))
	if !strings.Contains(out, `class="bb-code"`) || !strings.Contains(out, "plain text") {
		t.Errorf("plain fenced block missing chrome/content: %s", out)
	}
}

func TestChromaCSSGenerates(t *testing.T) {
	for _, style := range []string{"github", "github-dark"} {
		css, err := ChromaCSS(style)
		if err != nil {
			t.Fatalf("ChromaCSS(%q): %v", style, err)
		}
		if !strings.Contains(css, ".chroma") {
			t.Errorf("ChromaCSS(%q) missing .chroma rules", style)
		}
	}
}
