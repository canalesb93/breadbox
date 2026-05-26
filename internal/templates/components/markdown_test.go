//go:build !headless && !lite

package components

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func renderToString(t *testing.T, text string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Markdown(text).Render(context.Background(), &buf); err != nil {
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

func TestMarkdown_Paragraph(t *testing.T) {
	got := renderToString(t, "Hello world.\nSecond line.\n\nNew paragraph.")
	if !strings.Contains(got, `<p class="bb-prose-p">Hello world.<br/>Second line.</p>`) {
		t.Errorf("missing first paragraph in %q", got)
	}
	if !strings.Contains(got, `<p class="bb-prose-p">New paragraph.</p>`) {
		t.Errorf("missing second paragraph in %q", got)
	}
}

func TestMarkdown_EmphasisAndCode(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"This is **bold** text.", `<strong class="bb-prose-strong">bold</strong>`},
		{"This is *italic* text.", `<em class="bb-prose-em">italic</em>`},
		{"Try `inline` code.", `<code class="bb-prose-icode">inline</code>`},
		{"Mix `code with *not-italic*`.", `<code class="bb-prose-icode">code with *not-italic*</code>`},
		{"Snake_case_keep should not italicise.", `Snake_case_keep`},
	}
	for _, tc := range cases {
		got := renderToString(t, tc.in)
		if !strings.Contains(got, tc.want) {
			t.Errorf("for %q: missing %q in %q", tc.in, tc.want, got)
		}
	}
}

func TestMarkdown_Headers(t *testing.T) {
	got := renderToString(t, "## Section heading\n\nbody")
	if !strings.Contains(got, `<h3 class="bb-prose-h">Section heading</h3>`) {
		t.Errorf("missing h3 in %q", got)
	}
	got = renderToString(t, "### Sub heading")
	if !strings.Contains(got, `<h4 class="bb-prose-h">Sub heading</h4>`) {
		t.Errorf("missing h4 in %q", got)
	}
}

func TestMarkdown_FencedCode(t *testing.T) {
	src := "Before\n\n```go\nfmt.Println(\"hi\")\n```\n\nAfter"
	got := renderToString(t, src)
	if !strings.Contains(got, `<pre class="bb-prose-code" data-lang="go"><code>fmt.Println(&#34;hi&#34;)</code></pre>`) {
		t.Errorf("missing fenced code in %q", got)
	}
}

func TestMarkdown_List(t *testing.T) {
	got := renderToString(t, "- first\n- second\n- third")
	if !strings.Contains(got, `<ul class="bb-prose-ul"><li class="bb-prose-li">first</li><li class="bb-prose-li">second</li><li class="bb-prose-li">third</li></ul>`) {
		t.Errorf("unordered list shape wrong: %q", got)
	}
	got = renderToString(t, "1. first\n2. second")
	if !strings.Contains(got, `<ol class="bb-prose-ol"><li class="bb-prose-li">first</li><li class="bb-prose-li">second</li></ol>`) {
		t.Errorf("ordered list shape wrong: %q", got)
	}
}

func TestMarkdown_LinkSafety(t *testing.T) {
	good := renderToString(t, "Click [here](https://example.com).")
	if !strings.Contains(good, `<a href="https://example.com" class="bb-prose-link" rel="noopener noreferrer">here</a>`) {
		t.Errorf("safe link missing: %q", good)
	}
	bad := renderToString(t, "Click [here](javascript:alert(1)).")
	if strings.Contains(bad, "javascript") {
		t.Errorf("javascript scheme not stripped: %q", bad)
	}
}

func TestMarkdown_AutoLink(t *testing.T) {
	got := renderToString(t, "See https://example.com for more.")
	if !strings.Contains(got, `<a href="https://example.com" class="bb-prose-link"`) {
		t.Errorf("auto-link missing: %q", got)
	}
}

func TestMarkdown_HTMLEscaped(t *testing.T) {
	got := renderToString(t, "An <script>alert(1)</script> tag.")
	if strings.Contains(got, "<script>") {
		t.Errorf("script not escaped: %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("expected escaped form: %q", got)
	}
}

func TestMarkdown_Table(t *testing.T) {
	src := "| Col A | Col B |\n|---|---:|\n| 1 | 2.50 |\n| three | 4 |"
	got := renderToString(t, src)
	if !strings.Contains(got, `<table class="bb-prose-table">`) {
		t.Errorf("missing table tag: %q", got)
	}
	if !strings.Contains(got, `<th>Col A</th>`) {
		t.Errorf("missing first header: %q", got)
	}
	if !strings.Contains(got, `<td class="text-right">2.50</td>`) {
		t.Errorf("missing right-aligned cell: %q", got)
	}
}

func TestMarkdown_Blockquote(t *testing.T) {
	got := renderToString(t, "> quoted\n> still quoted")
	if !strings.Contains(got, `<blockquote class="bb-prose-quote">`) {
		t.Errorf("missing blockquote: %q", got)
	}
}
