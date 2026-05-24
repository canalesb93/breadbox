//go:build !headless && !lite

package admin

import (
	"net/http"

	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// DesignGalleryHandler serves GET /design — the design-system gallery.
// Hosts every component family on one anchored scroll for quick visual
// QA. Wired in the editor+ scope alongside other admin tooling.
func DesignGalleryHandler(sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := BaseTemplateData(r, sm, "design", "Design system")
		tr.RenderWithTempl(w, r, data, pages.DesignGallery(pages.DesignGalleryProps{
			Sections: pages.DesignSections(),
			Breadcrumbs: []components.Breadcrumb{
				{Label: "Design system"},
			},
		}))
	}
}

// DesignComponentHandler serves GET /design/c/{slug} — a single
// component family rendered in isolation. Same admin shell as the
// gallery (one-click back), but the main column hosts just one
// section so screenshots focus on the component under test.
func DesignComponentHandler(sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		section, ok := pages.FindDesignSection(slug)
		if !ok {
			tr.RenderNotFound(w, r)
			return
		}
		data := BaseTemplateData(r, sm, "design", section.Title)
		tr.RenderWithTempl(w, r, data, pages.DesignComponent(pages.DesignComponentProps{
			Section: section,
			Breadcrumbs: []components.Breadcrumb{
				{Label: "Design system", Href: "/design"},
				{Label: section.Title},
			},
		}))
	}
}
