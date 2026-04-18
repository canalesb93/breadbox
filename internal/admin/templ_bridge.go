package admin

import (
	"bytes"
	"html/template"
	"net/http"

	"github.com/a-h/templ"
)

// RenderTempl renders a templ.Component inside the existing html/template
// "base" layout so we can migrate pages incrementally without touching the
// shared chrome (nav, flash, mobile navbar). The component's HTML is rendered
// to a buffer, marked template-safe, and handed to a tiny host template that
// defines the "content" block as {{.TemplBody}}.
//
// `data` carries the usual BaseTemplateData fields (PageTitle, CurrentPage,
// Flash, CSRFToken, …) auto-injected by TemplateRenderer.Render. We simply
// add a TemplBody entry containing the pre-rendered component markup.
func (tr *TemplateRenderer) RenderTempl(w http.ResponseWriter, r *http.Request, component templ.Component, data map[string]any) {
	var buf bytes.Buffer
	if err := component.Render(r.Context(), &buf); err != nil {
		http.Error(w, "templ render error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	data["TemplBody"] = template.HTML(buf.String())
	tr.Render(w, r, "_templ_host.html", data)
}
