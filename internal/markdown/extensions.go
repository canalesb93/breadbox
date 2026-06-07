//go:build !headless && !lite

package markdown

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// This file holds the Breadbox-specific goldmark extensions layered on top
// of the standard ones:
//
//   - GitHub-style callouts/admonitions: > [!NOTE] / [!TIP] / [!IMPORTANT]
//     / [!WARNING] / [!CAUTION] render as colored, icon'd boxes.
//   - Heading hover-anchors: each heading with an auto id gets a "#"
//     deep-link revealed on hover.
//   - Code-block chrome: a header bar with the language label + a copy
//     button (the wrapper renderer; the copy click handler is code-copy.js).
//
// Markdown parsing, highlighting, and sanitization stay fully server-side.
// Icons use the app's standard `<i data-lucide>` placeholder (rendered by
// the global lucide runtime + MutationObserver in base.html) rather than
// inline SVG — bluemonday mangles SVG (it lowercases viewBox), and these
// glyphs are decorative chrome, not content.

// ─── Callouts ───────────────────────────────────────────────────────

const calloutAttr = "data-bb-callout"

var calloutRe = regexp.MustCompile(`(?i)^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]`)

type calloutMeta struct {
	label string
	icon  string // lucide icon name
}

var calloutTypes = map[string]calloutMeta{
	"note":      {"Note", "info"},
	"tip":       {"Tip", "lightbulb"},
	"important": {"Important", "message-square-warning"},
	"warning":   {"Warning", "triangle-alert"},
	"caution":   {"Caution", "octagon-alert"},
}

// calloutTransformer detects a blockquote whose first line is a callout
// marker ([!NOTE] etc.), tags the blockquote with its type, and strips the
// marker so only the body renders.
type calloutTransformer struct{}

func (calloutTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		bq, ok := n.(*ast.Blockquote)
		if !ok {
			return ast.WalkContinue, nil
		}
		para, ok := bq.FirstChild().(*ast.Paragraph)
		if !ok || para.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}
		// Detect from the raw first line — the inline parser splits "[!NOTE]"
		// into several Text nodes (the bracket is a link-opener), so matching
		// against parsed children is unreliable.
		firstLine := para.Lines().At(0)
		line := bytes.TrimSpace(firstLine.Value(source))
		m := calloutRe.FindSubmatch(line)
		if m == nil {
			return ast.WalkContinue, nil
		}
		// The marker must be alone on its line (GitHub semantics). If there's
		// trailing text on the same line it's an ordinary blockquote — leave it
		// untouched so the text isn't silently dropped.
		if len(bytes.TrimSpace(line[len(m[0]):])) != 0 {
			return ast.WalkContinue, nil
		}
		kind := strings.ToLower(string(m[1]))
		bq.SetAttributeString(calloutAttr, []byte(kind))

		// Strip the marker: remove every inline child on the marker's first
		// physical line. Anything below stays (the tight form keeps the body
		// in the same paragraph).
		var remove []ast.Node
		for c := para.FirstChild(); c != nil; c = c.NextSibling() {
			t, ok := c.(*ast.Text)
			if !ok || t.Segment.Start >= firstLine.Stop {
				break
			}
			remove = append(remove, c)
		}
		for _, c := range remove {
			para.RemoveChild(para, c)
		}
		// If the marker had its own paragraph (a blank line before the body),
		// drop the now-empty paragraph so it doesn't render a stray <p>.
		if para.FirstChild() == nil {
			bq.RemoveChild(bq, para)
		}
		return ast.WalkSkipChildren, nil
	})
}

// ─── Node renderer (callout blockquotes + heading anchors) ──────────

type bbNodeRenderer struct{}

func (r *bbNodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(ast.KindHeading, r.renderHeading)
}

func (r *bbNodeRenderer) renderBlockquote(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	kind := ""
	if v, ok := n.AttributeString(calloutAttr); ok {
		if b, ok := v.([]byte); ok {
			kind = string(b)
		}
	}
	if kind == "" {
		if entering {
			_, _ = w.WriteString("<blockquote>\n")
		} else {
			_, _ = w.WriteString("</blockquote>\n")
		}
		return ast.WalkContinue, nil
	}
	if entering {
		meta := calloutTypes[kind]
		_, _ = fmt.Fprintf(w, `<div class="bb-callout bb-callout-%s"><div class="bb-callout-title"><i data-lucide="%s" class="bb-callout-icon" aria-hidden="true"></i><span>%s</span></div><div class="bb-callout-body">`, kind, meta.icon, meta.label)
	} else {
		_, _ = w.WriteString("</div></div>\n")
	}
	return ast.WalkContinue, nil
}

func (r *bbNodeRenderer) renderHeading(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	h := n.(*ast.Heading)
	if entering {
		_, _ = fmt.Fprintf(w, "<h%d", h.Level)
		gmhtml.RenderAttributes(w, n, gmhtml.HeadingAttributeFilter)
		_ = w.WriteByte('>')
	} else {
		// Trailing hover-anchor: a link icon after the heading text, revealed on
		// hover. Trailing (not gutter) keeps the heading's left edge aligned
		// with paragraphs and avoids overflowing tight containers; the link
		// glyph reads as "copy link to this section".
		if id, ok := n.AttributeString("id"); ok {
			if b, ok := id.([]byte); ok {
				_, _ = fmt.Fprintf(w, `<a class="bb-heading-anchor" href="#%s" aria-label="Link to this section"><i data-lucide="link" class="bb-heading-anchor-icon" aria-hidden="true"></i></a>`, util.EscapeHTML(b))
			}
		}
		_, _ = fmt.Fprintf(w, "</h%d>\n", h.Level)
	}
	return ast.WalkContinue, nil
}

// ─── Code-block chrome (language pill + copy button) ────────────────

// codeWrapper is the highlighting WrapperRenderer. For highlighted blocks
// chroma emits the <pre class="chroma"> between the entering/exiting calls;
// for non-highlighted blocks the wrapper must emit the <pre><code> itself
// (goldmark-highlighting writes only the raw, escaped lines in that path).
func codeWrapper(w util.BufWriter, ctx highlighting.CodeBlockContext, entering bool) {
	highlighted := ctx.Highlighted()
	if entering {
		_, _ = w.WriteString(`<div class="bb-code"><div class="bb-code-bar">`)
		lang := ""
		if l, ok := ctx.Language(); ok && !isPlainLang(string(l)) {
			lang = string(util.EscapeHTML(l))
		}
		_, _ = fmt.Fprintf(w, `<span class="bb-code-lang">%s</span>`, lang)
		_, _ = w.WriteString(`<button type="button" class="bb-code-copy" data-bb-copy aria-label="Copy code"><i data-lucide="copy" class="bb-code-copy-icon" aria-hidden="true"></i><span class="bb-code-copy-label">Copy</span></button></div>`)
		if !highlighted {
			_, _ = w.WriteString(`<pre class="chroma"><code>`)
		}
	} else {
		if !highlighted {
			_, _ = w.WriteString("</code></pre>")
		}
		_, _ = w.WriteString("</div>\n")
	}
}

func isPlainLang(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "text", "plaintext", "fallback", "plain":
		return true
	}
	return false
}
