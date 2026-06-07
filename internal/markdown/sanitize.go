//go:build !headless && !lite

package markdown

import "github.com/microcosm-cc/bluemonday"

// policy is the HTML sanitizer applied to every goldmark output. goldmark
// already drops raw HTML from the source (WithUnsafe off); bluemonday is
// the second, authoritative line of defense and the place we whitelist the
// exact element/attribute shapes our renderer emits.
//
// Built once at init — *bluemonday.Policy is safe for concurrent use.
var policy = buildPolicy()

func buildPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()

	// Class attributes are needed for chroma syntax tokens
	// (<span class="k">…) and any prose styling hooks. Classes cannot
	// execute script, so allowing them globally is safe.
	p.AllowAttrs("class").Globally()

	// Auto-generated heading anchors (parser.WithAutoHeadingID).
	p.AllowAttrs("id").OnElements("h1", "h2", "h3", "h4", "h5", "h6")

	// chroma's classes-mode wrapper carries tabindex on <pre>.
	p.AllowAttrs("tabindex").OnElements("pre")

	// GFM tables — UGCPolicy does not allow table markup by default.
	p.AllowElements("table", "thead", "tbody", "tfoot", "tr", "th", "td", "caption")
	p.AllowAttrs("align").OnElements("th", "td")
	p.AllowAttrs("colspan", "rowspan").OnElements("th", "td")

	// Strikethrough.
	p.AllowElements("del", "s")

	// Read-only task-list checkboxes (extension.TaskList emits
	// <input checked="" disabled="" type="checkbox">).
	p.AllowElements("input")
	p.AllowAttrs("type", "checked", "disabled").OnElements("input")

	// External links open in a new tab and don't leak the referrer. Mirrors
	// what the old client-side markdown.js did after render.
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.RequireNoReferrerOnFullyQualifiedLinks(true)

	return p
}

func sanitize(html []byte) []byte {
	return policy.SanitizeBytes(html)
}
