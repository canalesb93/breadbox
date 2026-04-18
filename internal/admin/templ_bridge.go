package admin

import (
	"context"
	"html/template"
	"strings"

	"breadbox/internal/templates/components"

	"github.com/a-h/templ"
)

// renderComponent renders a registered templ component and returns its HTML as
// template.HTML so it can be emitted verbatim from html/template pages during
// the incremental templ migration (see issue #462). Unknown names expand to an
// HTML comment so stray calls don't crash a render.
func renderComponent(name string) template.HTML {
	fn, ok := templComponents[name]
	if !ok {
		return template.HTML("<!-- unknown templ component: " + template.HTMLEscapeString(name) + " -->")
	}
	var buf strings.Builder
	if err := fn().Render(context.Background(), &buf); err != nil {
		return template.HTML("<!-- templ render error: " + template.HTMLEscapeString(err.Error()) + " -->")
	}
	return template.HTML(buf.String())
}

// templComponents is the bridge registry. Every migrated zero-arg templ
// component gets a string alias so html/template callers can reach it via
// `{{renderComponent "skeleton-stat-cards"}}` until their own partial is
// ported. Drop an entry once every caller invokes the templ component
// directly.
var templComponents = map[string]func() templ.Component{
	"skeleton-stat-cards":       components.SkeletonStatCards,
	"skeleton-account-grid":     components.SkeletonAccountGrid,
	"skeleton-table-rows":       components.SkeletonTableRows,
	"skeleton-transaction-list": components.SkeletonTransactionList,
	"skeleton-sync-list":        components.SkeletonSyncList,
}
