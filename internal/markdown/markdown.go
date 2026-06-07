//go:build !headless && !lite

// Package markdown is the single server-side Markdown renderer for the
// admin UI. It replaces the old hand-rolled renderer and the client-side
// marked + DOMPurify stack: one parser, one sanitizer, one output shape
// rendered identically everywhere (agent transcripts, reports, comments,
// workflow prompt previews).
//
// Pipeline: goldmark (CommonMark + GFM + Typographer + chroma
// highlighting) → bluemonday sanitization. Raw HTML in the source is
// dropped by goldmark (WithUnsafe is OFF) and the output is sanitized as
// defense-in-depth, so untrusted agent/user content can never inject
// active markup.
//
// Output is a bare HTML fragment (no wrapping <div>); callers wrap it in
// the .bb-prose container — see internal/templates/components/markdown.go.
package markdown

import (
	"bytes"
	"html/template"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// md and mdBreaks are the two goldmark instances. They differ only in
// hard-wrap behavior: mdBreaks turns a single newline into <br>, which
// suits chat-style transaction comments (the old data-markdown-breaks).
// Built once at package init — goldmark.Markdown is safe for concurrent use.
var (
	md       = newGoldmark(false)
	mdBreaks = newGoldmark(true)
)

func newGoldmark(hardWraps bool) goldmark.Markdown {
	rendererOpts := []renderer.Option{
		gmhtml.WithXHTML(),
		// WithUnsafe is deliberately NOT set: raw HTML in the source is
		// dropped rather than passed through.
	}
	if hardWraps {
		rendererOpts = append(rendererOpts, gmhtml.WithHardWraps())
	}
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM, // tables + strikethrough + linkify + task lists
			extension.Typographer,
			highlighting.NewHighlighting(
				// Class-based output: chroma emits <span class="..."> tokens
				// themed by CSS (light + dark in input.css). No JS, no inline
				// styles, highlighted in the initial HTML.
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
		),
		goldmark.WithParserOptions(
			// Auto-generated heading anchors so long reports/transcripts get
			// linkable, scrollable sections. Heading levels are NOT shifted —
			// CSS (.bb-prose h1…h6) already keeps them visually modest, and
			// this matches the prior marked-based rendering.
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(rendererOpts...),
	)
}

// Render converts a Markdown string to a sanitized HTML fragment. Pass
// WithHardWraps() for chat-style content where single newlines should
// become line breaks.
func Render(src string, opts ...Option) template.HTML {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	cfg := config{}
	for _, o := range opts {
		o(&cfg)
	}
	engine := md
	if cfg.hardWraps {
		engine = mdBreaks
	}
	var buf bytes.Buffer
	if err := engine.Convert([]byte(src), &buf); err != nil {
		// Convert only errors on a broken writer/extension, not on bad
		// markdown — fall back to escaped plain text so content still shows.
		return template.HTML("<p>" + template.HTMLEscapeString(src) + "</p>") //nolint:gosec // escaped
	}
	return template.HTML(sanitize(buf.Bytes())) //nolint:gosec // bluemonday-sanitized
}

// RenderInline renders span-level Markdown only (emphasis, code, links,
// strikethrough) with no surrounding block element. Used for chip titles,
// table cells, and badge tooltips. Implemented by rendering normally and
// unwrapping a single leading <p>…</p>.
func RenderInline(src string) template.HTML {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	out := strings.TrimSpace(string(Render(src)))
	// A single-paragraph result unwraps cleanly to span-level content.
	if strings.HasPrefix(out, "<p>") && strings.HasSuffix(out, "</p>") &&
		strings.Count(out, "<p>") == 1 {
		out = strings.TrimSuffix(strings.TrimPrefix(out, "<p>"), "</p>")
	}
	return template.HTML(out) //nolint:gosec // already sanitized by Render
}

// Option configures a Render call.
type Option func(*config)

type config struct {
	hardWraps bool
}

// WithHardWraps makes a single newline render as <br> (chat-style).
func WithHardWraps() Option { return func(c *config) { c.hardWraps = true } }
