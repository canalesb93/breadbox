//go:build !headless && !lite

package components

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

// JSONViewerProps drives the JSONViewer component.
type JSONViewerProps struct {
	// Source is either a JSON string (already a serialised payload, the
	// common case for tool inputs/results coming out of the SDK) or any
	// Go value that can be marshalled via encoding/json. When both are
	// non-zero, Source takes priority.
	Source string
	Value  any

	// Class is appended to the root <div>. Use sparingly — the component
	// already ships its own surface styling via .bb-json.
	Class string

	// MaxDepth controls how many levels render expanded by default. Zero
	// means "expand everything"; 1 means "only the top-level object/array
	// keys, descendants start collapsed". Defaults to 0.
	MaxDepth int

	// ShowCopy adds a small "Copy" button (Alpine) in the top-right
	// corner that copies the raw JSON to the clipboard.
	ShowCopy bool

	// Empty is the placeholder rendered when Source/Value is empty.
	// Defaults to "—".
	Empty string
}

// JSONViewer is a server-rendered, syntax-highlighted JSON pretty-printer.
//
// Objects and arrays render as <details> blocks so operators can collapse
// noisy subtrees. Token classes (`bb-json-key`, `-string`, `-num`,
// `-bool`, `-null`, `-punct`) are colored by the design-system CSS — no
// JS highlighter dependency.
//
// When ShowCopy is true the viewer is wrapped in a tiny Alpine factory
// (`bbJsonViewer`) that exposes a Copy button. The copy text is the raw
// (un-prettified) Source, so paste-into-another-tool fidelity is
// preserved.
func JSONViewer(p JSONViewerProps) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		raw := strings.TrimSpace(p.Source)
		if raw == "" && p.Value == nil {
			empty := p.Empty
			if empty == "" {
				empty = "—"
			}
			_, err := fmt.Fprintf(w, `<span class="text-base-content/40">%s</span>`, html.EscapeString(empty))
			return err
		}

		// Parse so we can pretty-print. If parsing fails, fall back to a
		// plain <pre> so the operator still sees something.
		var parsed any
		var parseErr error
		if raw != "" {
			parseErr = json.Unmarshal([]byte(raw), &parsed)
		} else {
			parsed = p.Value
			// Also marshal the raw form for the copy button.
			if b, err := json.Marshal(p.Value); err == nil {
				raw = string(b)
			}
		}
		if parseErr != nil {
			_, err := fmt.Fprintf(w,
				`<pre class="bb-json bb-json--raw text-xs font-mono whitespace-pre-wrap break-words overflow-x-auto">%s</pre>`,
				html.EscapeString(raw),
			)
			return err
		}

		var b strings.Builder
		classes := "bb-json"
		if c := strings.TrimSpace(p.Class); c != "" {
			classes += " " + c
		}

		if p.ShowCopy {
			b.WriteString(`<div class="bb-json-wrap" x-data="bbJsonViewer">`)
			b.WriteString(`<button type="button" class="bb-json-copy" @click="copyJson($refs.payload)" :class="copied ? 'is-copied' : ''" :title="copied ? 'Copied' : 'Copy JSON'">`)
			b.WriteString(`<i data-lucide="clipboard" class="w-3.5 h-3.5" x-show="!copied"></i>`)
			b.WriteString(`<i data-lucide="check" class="w-3.5 h-3.5" x-show="copied" x-cloak></i>`)
			b.WriteString(`</button>`)
			b.WriteString(`<script type="application/json" x-ref="payload">`)
			b.WriteString(html.EscapeString(raw))
			b.WriteString(`</script>`)
		}

		b.WriteString(`<div class="`)
		b.WriteString(classes)
		b.WriteString(`">`)
		renderJSONValue(&b, parsed, 0, p.MaxDepth)
		b.WriteString(`</div>`)

		if p.ShowCopy {
			b.WriteString(`</div>`)
		}

		_, err := io.WriteString(w, b.String())
		return err
	})
}

// renderJSONValue dispatches on the JSON value's Go-side type and emits
// the matching token spans / <details> tree. depth is the current
// nesting level; maxDepth (when >0) collapses all <details> at or below
// that level on first paint.
func renderJSONValue(b *strings.Builder, v any, depth, maxDepth int) {
	switch t := v.(type) {
	case nil:
		b.WriteString(`<span class="bb-json-null">null</span>`)
	case bool:
		b.WriteString(`<span class="bb-json-bool">`)
		if t {
			b.WriteString(`true`)
		} else {
			b.WriteString(`false`)
		}
		b.WriteString(`</span>`)
	case float64:
		b.WriteString(`<span class="bb-json-num">`)
		b.WriteString(formatJSONNumber(t))
		b.WriteString(`</span>`)
	case json.Number:
		b.WriteString(`<span class="bb-json-num">`)
		b.WriteString(string(t))
		b.WriteString(`</span>`)
	case string:
		b.WriteString(`<span class="bb-json-string">"`)
		b.WriteString(escapeJSONString(t))
		b.WriteString(`"</span>`)
	case []any:
		renderJSONArray(b, t, depth, maxDepth)
	case map[string]any:
		renderJSONObject(b, t, depth, maxDepth)
	default:
		// Fallback: marshal and dump as raw escaped text.
		out, err := json.Marshal(v)
		if err != nil {
			b.WriteString(`<span class="bb-json-null">?</span>`)
			return
		}
		b.WriteString(`<span class="bb-json-string">`)
		b.WriteString(html.EscapeString(string(out)))
		b.WriteString(`</span>`)
	}
}

func renderJSONArray(b *strings.Builder, arr []any, depth, maxDepth int) {
	if len(arr) == 0 {
		b.WriteString(`<span class="bb-json-punct">[]</span>`)
		return
	}
	open := depthIsOpen(depth, maxDepth)
	b.WriteString(`<details class="bb-json-block"`)
	if open {
		b.WriteString(` open`)
	}
	b.WriteString(`><summary class="bb-json-summary"><span class="bb-json-punct">[</span>`)
	b.WriteString(`<span class="bb-json-collapsed">… ` + strconv.Itoa(len(arr)) + ` items</span>`)
	b.WriteString(`<span class="bb-json-trailing-punct">]</span></summary>`)
	b.WriteString(`<div class="bb-json-children">`)
	for i, item := range arr {
		b.WriteString(`<div class="bb-json-row">`)
		renderJSONValue(b, item, depth+1, maxDepth)
		if i < len(arr)-1 {
			b.WriteString(`<span class="bb-json-punct">,</span>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<div class="bb-json-close"><span class="bb-json-punct">]</span></div>`)
	b.WriteString(`</details>`)
}

func renderJSONObject(b *strings.Builder, obj map[string]any, depth, maxDepth int) {
	if len(obj) == 0 {
		b.WriteString(`<span class="bb-json-punct">{}</span>`)
		return
	}
	keys := sortedObjectKeys(obj)
	open := depthIsOpen(depth, maxDepth)
	b.WriteString(`<details class="bb-json-block"`)
	if open {
		b.WriteString(` open`)
	}
	b.WriteString(`><summary class="bb-json-summary"><span class="bb-json-punct">{</span>`)
	b.WriteString(`<span class="bb-json-collapsed">… ` + strconv.Itoa(len(keys)) + ` keys</span>`)
	b.WriteString(`<span class="bb-json-trailing-punct">}</span></summary>`)
	b.WriteString(`<div class="bb-json-children">`)
	for i, k := range keys {
		b.WriteString(`<div class="bb-json-row">`)
		b.WriteString(`<span class="bb-json-key">"`)
		b.WriteString(escapeJSONString(k))
		b.WriteString(`"</span>`)
		b.WriteString(`<span class="bb-json-punct">: </span>`)
		renderJSONValue(b, obj[k], depth+1, maxDepth)
		if i < len(keys)-1 {
			b.WriteString(`<span class="bb-json-punct">,</span>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<div class="bb-json-close"><span class="bb-json-punct">}</span></div>`)
	b.WriteString(`</details>`)
}

// sortedObjectKeys preserves a stable order even though json.Unmarshal
// emits Go maps with non-deterministic iteration. We use insertion-order
// when the source was a `json.Number`-aware decoder; absent that, fall
// back to alphabetical so the rendered tree is stable across reloads.
func sortedObjectKeys(obj map[string]any) []string {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	// Alphabetical — the tool I/O payloads we render here are small
	// enough that this isn't a perf concern, and predictability beats
	// surprise.
	sortStrings(keys)
	return keys
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

func depthIsOpen(depth, maxDepth int) bool {
	if maxDepth <= 0 {
		return true
	}
	return depth < maxDepth
}

// formatJSONNumber removes trailing zeros / unnecessary decimals so
// `1.0` doesn't render as `1.000000` after float64 round-trip.
func formatJSONNumber(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// escapeJSONString HTML-escapes a JSON string value AND converts common
// control characters back into their `\n`/`\t`/`\r` escape forms so the
// pretty-print looks like the source.
func escapeJSONString(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '<':
			b.WriteString(`&lt;`)
		case '>':
			b.WriteString(`&gt;`)
		case '&':
			b.WriteString(`&amp;`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
