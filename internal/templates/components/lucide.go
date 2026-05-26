//go:build !headless && !lite

package components

import "html/template"

// LucideFuncMap returns the html/template func registration that mirrors
// the @LucideIcon templ component for legacy html/template pages (currently
// just internal/templates/layout/base.html).
//
// Usage in templates:
//
//	{{ lucide "home" "w-4 h-4" }}
//
// Pass "" for class when no extra classes are needed.
func LucideFuncMap() template.FuncMap {
	return template.FuncMap{
		"lucide": func(name, class string) template.HTML {
			return template.HTML(renderLucideIcon(name, class, ""))
		},
	}
}
