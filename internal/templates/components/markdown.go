//go:build !headless && !lite

package components

import (
	"context"
	"io"

	"breadbox/internal/markdown"

	"github.com/a-h/templ"
)

// Markdown renders a Markdown string as a safe HTML fragment wrapped in
// <div class="bb-prose">. Parsing, GFM extensions, syntax highlighting, and
// sanitization all live in internal/markdown — this is the templ-facing
// adapter so call sites write @components.Markdown(text).
//
// This single server-side renderer replaced both the old hand-rolled Go
// renderer and the client-side marked + DOMPurify stack, so transcripts,
// reports, comments, and prompt previews now produce identical HTML.
func Markdown(text string) templ.Component {
	return proseComponent(text, "")
}

// MarkdownLarge is Markdown with the .bb-prose-lg size modifier — used for
// full-page report bodies that want more generous spacing than in-bubble
// transcript text.
func MarkdownLarge(text string) templ.Component {
	return proseComponent(text, " bb-prose-lg")
}

// MarkdownBreaks renders chat-style Markdown where a single newline becomes
// a <br> (the old data-markdown-breaks="true"). Used for transaction
// comments.
func MarkdownBreaks(text string) templ.Component {
	return proseComponent(text, "", markdown.WithHardWraps())
}

func proseComponent(text, extraClass string, opts ...markdown.Option) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		html := markdown.Render(text, opts...)
		if html == "" {
			return nil
		}
		_, err := io.WriteString(w, `<div class="bb-prose`+extraClass+`">`+string(html)+`</div>`)
		return err
	})
}

// MarkdownInline renders span-level Markdown only (emphasis, code, links,
// strikethrough) with no surrounding block element — for chip titles, table
// cells, and badge tooltips.
func MarkdownInline(text string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		html := markdown.RenderInline(text)
		if html == "" {
			return nil
		}
		_, err := io.WriteString(w, string(html))
		return err
	})
}
