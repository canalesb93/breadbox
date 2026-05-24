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
		data["Standalone"] = true
		tr.RenderWithTempl(w, r, data, pages.DesignGallery(pages.DesignGalleryProps{
			Sections: pages.DesignSections(),
			Breadcrumbs: []components.Breadcrumb{
				{Label: "Design system"},
			},
		}))
	}
}

// DesignComponentHandler serves GET /design/c/{slug} — a single
// component family rendered in isolation. Two render modes:
//
//   - Default: the standalone shell hosts an iframe pointing at the
//     `?embed=1` variant of this same route, so the viewport toggle
//     can resize the iframe and trigger real Tailwind responsive
//     breakpoints (sm:, lg:, etc.) which key off viewport width.
//   - `?embed=1`: a bare HTML shell with just the section's demo
//     markup. Loaded inside the iframe above. No sidebar, no top bar,
//     no breadcrumb — only the global head scripts (Alpine, Lucide,
//     daisy CSS) so the demo's interactivity still works.
func DesignComponentHandler(sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		section, ok := pages.FindDesignSection(slug)
		if !ok {
			tr.RenderNotFound(w, r)
			return
		}
		data := BaseTemplateData(r, sm, "design", section.Title)
		if r.URL.Query().Get("embed") == "1" {
			data["Embed"] = true
			tr.RenderWithTempl(w, r, data, pages.DesignComponentEmbed(pages.DesignComponentProps{
				Section: section,
			}))
			return
		}
		data["Standalone"] = true
		// Drop the standalone shell's max-w-6xl cap so the viewport
		// toggle's Free mode (≥1320px so xl: utilities trigger) has
		// room to grow without forcing a body horizontal scroll on
		// reasonable browser widths.
		data["WideMain"] = true
		tr.RenderWithTempl(w, r, data, pages.DesignComponent(pages.DesignComponentProps{
			Section: section,
			Breadcrumbs: []components.Breadcrumb{
				{Label: "Design system", Href: "/design"},
				{Label: section.Title},
			},
		}))
	}
}
