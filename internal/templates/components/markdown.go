//go:build !headless && !lite

package components

import (
	"context"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/a-h/templ"
)

// Markdown renders a compact subset of CommonMark + a few GFM extensions
// as a safe HTML fragment. Used by the agent transcript to render
// assistant text the way most operators read it in Claude Code (lists,
// code blocks, links, occasional tables) without shipping a full
// markdown engine.
//
// Supported:
//   - Paragraphs split by blank lines
//   - ATX headers ##, ### (h1 is reserved for the page; we don't render #)
//   - **bold**, *italic*, `inline code`
//   - Fenced code blocks ```lang
//   - Unordered (- * +) and ordered (1.) lists with one level of nesting
//   - Blockquotes (>) and ---/*** horizontal rules
//   - [text](url) and bare http(s)://… links (only http/https/mailto)
//   - GFM tables: header row + delimiter + body rows
//
// Not supported (deliberately): raw HTML passthrough, footnotes, definition
// lists, image embeds, task list checkboxes (yet). Everything outside the
// supported syntax is rendered as escaped plain text — never as raw HTML.
//
// Output is wrapped in <div class="bb-prose"> so callers can style the
// rendered tree via a single rule set without bleeding into surrounding
// page text.
func Markdown(text string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		_, err := io.WriteString(w, `<div class="bb-prose">`+renderMarkdown(text)+`</div>`)
		return err
	})
}

// MarkdownInline renders text with span-level markdown only (no
// paragraphs, headers, lists, or block elements). Useful for chip
// titles, table cells, badge tooltips, or anywhere a single line should
// keep its inline emphasis without expanding into a block element tree.
func MarkdownInline(text string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		s := strings.TrimSpace(text)
		if s == "" {
			return nil
		}
		_, err := io.WriteString(w, renderInline(s))
		return err
	})
}

// renderMarkdown is the block-level dispatcher. We hand-roll a tiny
// line-driven state machine rather than building a full AST because the
// agent transcript only emits a handful of block kinds and we want to
// stay zero-dep.
func renderMarkdown(text string) string {
	// Normalise line endings + strip a trailing newline so the last
	// paragraph doesn't render as an extra empty block.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	lines := strings.Split(text, "\n")
	var out strings.Builder

	for i := 0; i < len(lines); i++ {
		ln := lines[i]

		// Fenced code block — capture until the closing ``` on its own
		// line. Language hint after the opening fence is preserved as a
		// data attribute so a future highlighter can pick it up; the
		// content itself is escaped, never executed as markdown.
		if isFenceLine(ln) {
			lang := strings.TrimSpace(strings.TrimLeft(ln, "`~"))
			j := i + 1
			var body strings.Builder
			for j < len(lines) && !isFenceLine(lines[j]) {
				if body.Len() > 0 {
					body.WriteByte('\n')
				}
				body.WriteString(lines[j])
				j++
			}
			out.WriteString(`<pre class="bb-prose-code" data-lang="`)
			out.WriteString(html.EscapeString(lang))
			out.WriteString(`"><code>`)
			out.WriteString(html.EscapeString(body.String()))
			out.WriteString(`</code></pre>`)
			// Skip past the closing fence (or end-of-input).
			i = j
			continue
		}

		// Blank line → block boundary.
		if strings.TrimSpace(ln) == "" {
			continue
		}

		// Horizontal rule.
		if isHRLine(ln) {
			out.WriteString(`<hr class="bb-prose-hr"/>`)
			continue
		}

		// ATX headers — h1 is reserved for page chrome so we down-rank:
		// # → h3, ## → h3, ### → h4. Keeps the visual hierarchy below
		// the page's <h1> regardless of how the agent prefixes.
		if h, body, ok := atxHeader(ln); ok {
			tag := "h3"
			if h >= 3 {
				tag = "h4"
			}
			out.WriteString(`<` + tag + ` class="bb-prose-h">`)
			out.WriteString(renderInline(body))
			out.WriteString(`</` + tag + `>`)
			continue
		}

		// Blockquote — collect consecutive `> ` lines, render their
		// (de-quoted) bodies recursively so nested emphasis still works.
		if strings.HasPrefix(strings.TrimLeft(ln, " "), ">") {
			j := i
			var inner strings.Builder
			for j < len(lines) {
				stripped := strings.TrimLeft(lines[j], " ")
				if !strings.HasPrefix(stripped, ">") {
					break
				}
				if inner.Len() > 0 {
					inner.WriteByte('\n')
				}
				inner.WriteString(strings.TrimPrefix(strings.TrimPrefix(stripped, ">"), " "))
				j++
			}
			out.WriteString(`<blockquote class="bb-prose-quote">`)
			out.WriteString(renderMarkdown(inner.String()))
			out.WriteString(`</blockquote>`)
			i = j - 1
			continue
		}

		// List (ordered or unordered).
		if isListItem(ln) {
			rendered, consumed := renderList(lines[i:])
			out.WriteString(rendered)
			i += consumed - 1
			continue
		}

		// GFM table — must be two header rows minimum (header + delimiter).
		if i+1 < len(lines) && isTableDelimiter(lines[i+1]) && strings.Contains(ln, "|") {
			rendered, consumed := renderTable(lines[i:])
			if consumed > 0 {
				out.WriteString(rendered)
				i += consumed - 1
				continue
			}
		}

		// Paragraph — accumulate consecutive non-blank, non-block lines.
		j := i
		var para strings.Builder
		for j < len(lines) {
			lj := lines[j]
			if strings.TrimSpace(lj) == "" || isFenceLine(lj) || isHRLine(lj) || isListItem(lj) || strings.HasPrefix(strings.TrimLeft(lj, " "), ">") {
				break
			}
			if _, _, ok := atxHeader(lj); ok {
				break
			}
			if para.Len() > 0 {
				para.WriteByte('\n')
			}
			para.WriteString(lj)
			j++
		}
		out.WriteString(`<p class="bb-prose-p">`)
		out.WriteString(renderInline(para.String()))
		out.WriteString(`</p>`)
		i = j - 1
	}
	return out.String()
}

// renderList consumes a contiguous block of list items (siblings at the
// same indent level). Returns the rendered HTML + the number of lines
// consumed. Handles one level of nesting via leading-whitespace runs of
// 2+ spaces or a tab.
func renderList(lines []string) (string, int) {
	ordered := isOrderedItem(lines[0])
	var out strings.Builder
	if ordered {
		out.WriteString(`<ol class="bb-prose-ol">`)
	} else {
		out.WriteString(`<ul class="bb-prose-ul">`)
	}

	i := 0
	for i < len(lines) {
		ln := lines[i]
		trimmed := strings.TrimLeft(ln, " \t")
		// Stop at the first line that's neither a sibling item nor an
		// indented continuation / nested item.
		if !isListItem(trimmed) || (ordered && !isOrderedItem(trimmed)) || (!ordered && isOrderedItem(trimmed)) {
			break
		}
		// Detect the marker length so the body extracted below skips it
		// regardless of whether the marker is `- `, `* `, or `12. `.
		body := stripListMarker(trimmed)

		// Capture continuation lines (indented by at least 2 spaces) and
		// any nested list block.
		j := i + 1
		var nested []string
		var cont strings.Builder
		for j < len(lines) {
			lj := lines[j]
			if strings.TrimSpace(lj) == "" {
				break
			}
			leading := leadingSpaces(lj)
			if leading >= 2 {
				if isListItem(strings.TrimLeft(lj, " \t")) {
					nested = append(nested, strings.TrimPrefix(lj, "  "))
					j++
					continue
				}
				if cont.Len() > 0 {
					cont.WriteByte('\n')
				}
				cont.WriteString(strings.TrimLeft(lj, " \t"))
				j++
				continue
			}
			break
		}

		out.WriteString(`<li class="bb-prose-li">`)
		out.WriteString(renderInline(body))
		if cont.Len() > 0 {
			// Render continuation prose as a tight follow-up <p>.
			out.WriteString(`<p class="bb-prose-li-cont">`)
			out.WriteString(renderInline(cont.String()))
			out.WriteString(`</p>`)
		}
		if len(nested) > 0 {
			n, _ := renderList(nested)
			out.WriteString(n)
		}
		out.WriteString(`</li>`)
		i = j
	}

	if ordered {
		out.WriteString(`</ol>`)
	} else {
		out.WriteString(`</ul>`)
	}
	return out.String(), i
}

// renderTable consumes a GFM table starting at lines[0]. Returns ("", 0)
// when the candidate block doesn't actually parse as a table — the caller
// then falls through to the paragraph branch.
func renderTable(lines []string) (string, int) {
	if len(lines) < 2 {
		return "", 0
	}
	headers := splitTableRow(lines[0])
	if len(headers) == 0 {
		return "", 0
	}
	aligns := parseTableAligns(lines[1])
	if len(aligns) != len(headers) {
		return "", 0
	}

	var out strings.Builder
	out.WriteString(`<div class="bb-prose-table-wrap"><table class="bb-prose-table"><thead><tr>`)
	for i, h := range headers {
		out.WriteString(`<th`)
		if i < len(aligns) && aligns[i] != "" {
			out.WriteString(` class="text-` + aligns[i] + `"`)
		}
		out.WriteString(`>`)
		out.WriteString(renderInline(h))
		out.WriteString(`</th>`)
	}
	out.WriteString(`</tr></thead><tbody>`)

	consumed := 2
	for i := 2; i < len(lines); i++ {
		row := splitTableRow(lines[i])
		if len(row) == 0 {
			break
		}
		out.WriteString(`<tr>`)
		for j, cell := range row {
			out.WriteString(`<td`)
			if j < len(aligns) && aligns[j] != "" {
				out.WriteString(` class="text-` + aligns[j] + `"`)
			}
			out.WriteString(`>`)
			out.WriteString(renderInline(cell))
			out.WriteString(`</td>`)
		}
		out.WriteString(`</tr>`)
		consumed++
	}

	out.WriteString(`</tbody></table></div>`)
	return out.String(), consumed
}

// renderInline handles span-level markdown within a block. We extract
// inline-code spans and `[text](url)` links into placeholders BEFORE
// HTML escaping and emphasis run, so:
//   - inline code is not re-parsed for emphasis,
//   - explicit links are not re-wrapped by the bare-URL auto-linker.
// The placeholders are stitched back in at the end after emphasis +
// auto-link.
func renderInline(s string) string {
	// Step 1: pluck out backtick code spans into placeholders.
	codePlaceholders := []string{}
	s = inlineCodeRe.ReplaceAllStringFunc(s, func(m string) string {
		body := strings.Trim(m, "`")
		codePlaceholders = append(codePlaceholders, body)
		return inlineCodePlaceholder(len(codePlaceholders) - 1)
	})

	// Step 2: pluck explicit links `[text](url)` into placeholders. We
	// keep the parsed label + href next to each placeholder so we can
	// emit the final anchor with already-escaped contents at restore time.
	linkPlaceholders := []renderedInlineLink{}
	s = inlineLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := inlineLinkRe.FindStringSubmatch(m)
		if len(sub) != 3 {
			return m
		}
		href := safeHref(sub[2])
		if href == "" {
			// Drop the link, keep the visible label as plain text.
			return sub[1]
		}
		linkPlaceholders = append(linkPlaceholders, renderedInlineLink{Label: sub[1], Href: href})
		return inlineLinkPlaceholder(len(linkPlaceholders) - 1)
	})

	// Step 3: escape any remaining HTML metacharacters in the text.
	s = html.EscapeString(s)

	// Step 4: emphasis (bold, italic, strike).
	s = emphasisOnly(s)

	// Step 5: bare-URL auto-link. Safe now — explicit links + inline code
	// are placeholders, so the regex cannot match a URL already wrapped
	// in an <a> tag.
	s = autoLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		href := safeHref(m)
		if href == "" {
			return m
		}
		return `<a href="` + href + `" class="bb-prose-link" rel="noopener noreferrer">` + m + `</a>`
	})

	// Step 6: restore link placeholders. The label is emphasis-rendered
	// (inline emphasis like **bold** inside [**bold**](url) should still
	// work) and HTML-escaped — note that the label may contain its own
	// inline-code placeholders, so we leave the final code-placeholder
	// restoration for step 7 below.
	for i, lp := range linkPlaceholders {
		label := emphasisOnly(html.EscapeString(lp.Label))
		anchor := `<a href="` + lp.Href + `" class="bb-prose-link" rel="noopener noreferrer">` + label + `</a>`
		s = strings.Replace(s, inlineLinkPlaceholder(i), anchor, 1)
	}

	// Step 7: restore inline-code placeholders.
	for idx, body := range codePlaceholders {
		s = strings.Replace(s, inlineCodePlaceholder(idx), `<code class="bb-prose-icode">`+html.EscapeString(body)+`</code>`, 1)
	}

	// Step 8: soft line break inside a paragraph renders as <br>.
	s = strings.ReplaceAll(s, "\n", "<br/>")
	return s
}

type renderedInlineLink struct {
	Label string
	Href  string
}

func inlineLinkPlaceholder(idx int) string {
	return "\x00MD-INLINELINK-" + indexToken(idx) + "\x00"
}

// emphasisOnly applies bold + italic + strikethrough to an already-escaped
// string. Split out so renderInline can run it twice: once on the body of
// a link label (which is fed in HTML-escaped already) and once on the
// outer string.
//
// Italic uses leading/trailing context-character capture groups (`$1` /
// `$3`) because Go's regex engine has no lookbehind — preserving the
// surrounding whitespace/punctuation is the only way to avoid mangling
// `snake_case` identifiers while still catching `*emphasis*` inside a
// sentence.
func emphasisOnly(s string) string {
	s = boldStarsRe.ReplaceAllString(s, `<strong class="bb-prose-strong">$1</strong>`)
	s = boldUnderRe.ReplaceAllString(s, `<strong class="bb-prose-strong">$1</strong>`)
	s = italicStarsRe.ReplaceAllString(s, `$1<em class="bb-prose-em">$2</em>$3`)
	s = italicUnderRe.ReplaceAllString(s, `$1<em class="bb-prose-em">$2</em>$3`)
	s = strikeRe.ReplaceAllString(s, `<s class="bb-prose-strike">$1</s>`)
	return s
}

// ─── line classification helpers ───────────────────────────────────

func isFenceLine(s string) bool {
	t := strings.TrimSpace(s)
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

func isHRLine(s string) bool {
	t := strings.TrimSpace(s)
	if len(t) < 3 {
		return false
	}
	c := t[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	for _, r := range t {
		if byte(r) != c {
			return false
		}
	}
	return true
}

func atxHeader(s string) (int, string, bool) {
	t := strings.TrimLeft(s, " ")
	level := 0
	for level < len(t) && t[level] == '#' && level < 6 {
		level++
	}
	if level == 0 {
		return 0, "", false
	}
	if level >= len(t) || t[level] != ' ' {
		return 0, "", false
	}
	return level, strings.TrimRight(strings.TrimSpace(t[level+1:]), "#"), true
}

func isListItem(s string) bool {
	t := strings.TrimLeft(s, " \t")
	if len(t) < 2 {
		return false
	}
	if (t[0] == '-' || t[0] == '*' || t[0] == '+') && t[1] == ' ' {
		return true
	}
	return isOrderedItem(t)
}

func isOrderedItem(s string) bool {
	t := strings.TrimLeft(s, " \t")
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(t) {
		return false
	}
	if t[i] != '.' && t[i] != ')' {
		return false
	}
	if i+1 >= len(t) || t[i+1] != ' ' {
		return false
	}
	return true
}

func stripListMarker(s string) string {
	if len(s) > 1 && (s[0] == '-' || s[0] == '*' || s[0] == '+') && s[1] == ' ' {
		return s[2:]
	}
	// ordered: skip digits, dot/paren, space.
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i < len(s) && (s[i] == '.' || s[i] == ')') {
		i++
	}
	if i < len(s) && s[i] == ' ' {
		i++
	}
	return s[i:]
}

func leadingSpaces(s string) int {
	n := 0
	for _, r := range s {
		if r == ' ' {
			n++
		} else if r == '\t' {
			n += 4
		} else {
			break
		}
	}
	return n
}

func isTableDelimiter(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" || !strings.ContainsAny(t, "-:") {
		return false
	}
	parts := splitTableRow(t)
	if len(parts) == 0 {
		return false
	}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if !tableDelimRe.MatchString(p) {
			return false
		}
	}
	return true
}

func splitTableRow(s string) []string {
	t := strings.TrimSpace(s)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	if t == "" {
		return nil
	}
	parts := strings.Split(t, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func parseTableAligns(s string) []string {
	parts := splitTableRow(s)
	out := make([]string, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		left := strings.HasPrefix(p, ":")
		right := strings.HasSuffix(p, ":")
		switch {
		case left && right:
			out[i] = "center"
		case right:
			out[i] = "right"
		case left:
			out[i] = "left"
		default:
			out[i] = ""
		}
	}
	return out
}

// safeHref accepts a URL string and returns it (without sanitisation,
// since the user-facing surface already prevents JS schemes) or "" if
// the scheme is something we refuse to link to. Used by both [text](url)
// links and bare-URL auto-links.
func safeHref(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		// Treat bare paths like /foo and #anchor as same-origin links.
		if strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "#") {
			return html.EscapeString(raw)
		}
		return ""
	}
	switch scheme {
	case "http", "https", "mailto":
		return html.EscapeString(raw)
	}
	return ""
}

func inlineCodePlaceholder(idx int) string {
	// Sentinel chosen so it cannot collide with normal text. We add the
	// idx as a digit suffix so multiple code spans round-trip distinctly.
	return "\x00MD-INLINECODE-" + indexToken(idx) + "\x00"
}

func indexToken(n int) string {
	// Cheap base36-ish encoder; idx fits comfortably in a 32-bit int.
	if n == 0 {
		return "0"
	}
	const alpha = "0123456789abcdefghijklmnopqrstuvwxyz"
	var b []byte
	for n > 0 {
		b = append([]byte{alpha[n%36]}, b...)
		n /= 36
	}
	return string(b)
}

// ─── regexes ───────────────────────────────────────────────────────

var (
	inlineCodeRe = regexp.MustCompile("`[^`\n]+`")
	inlineLinkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	// Bold + italic are non-greedy and require the delimiter not to be
	// preceded/followed by a word character on the wrong side (so
	// snake_case_variable is not mangled).
	boldStarsRe   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	boldUnderRe   = regexp.MustCompile(`__([^_]+)__`)
	italicStarsRe = regexp.MustCompile(`(^|[\s(])\*([^*\n]+)\*($|[\s).,!?;:])`)
	italicUnderRe = regexp.MustCompile(`(^|[\s(])_([^_\n]+)_($|[\s).,!?;:])`)
	strikeRe      = regexp.MustCompile(`~~([^~]+)~~`)
	autoLinkRe    = regexp.MustCompile(`https?://[^\s<>"]+[^\s<>".,!?;:)]`)
	tableDelimRe  = regexp.MustCompile(`^:?-{1,}:?$`)
)
