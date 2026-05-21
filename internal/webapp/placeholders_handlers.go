//go:build !headless && !lite

package webapp

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/webapp/pages"
)

// registerPlaceholders wires "coming soon" pages for nav leaves not yet built.
// Each is a real route inside the shell so navigation stays usable.
func (h *Handler) registerPlaceholders(r chi.Router) {
	r.Get("/reports", h.placeholder("Reports", "Spending reports and trends over time are coming to v3 soon."))
	r.Get("/reviews", h.placeholder("Reviews", "Your transaction review queue will live here soon."))
	r.Get("/insights", h.placeholder("Insights", "Charts and insights into your household finances are on the way."))
	r.Get("/settings", h.placeholder("Settings", "Household and app settings are coming to v3 soon."))
}

// placeholder returns a handler that renders the coming-soon page with the given
// title and message.
func (h *Handler) placeholder(title, message string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		render(w, r, http.StatusOK, pages.Placeholder(h.shellData(r, title), title, message))
	}
}
