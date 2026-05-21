//go:build headless

// Headless builds exclude the dashboard. This stub keeps internal/api able to import
// and reference webapp.New(...).Router() unconditionally (matching the admin/webui
// pattern); the runtime --no-dashboard gate skips mounting it anyway.
package webapp

import (
	"net/http"

	"github.com/alexedwards/scs/v2"

	"breadbox/internal/app"
)

// Handler is the headless no-op stand-in for the webapp subrouter.
type Handler struct{}

// New returns a stub handler under -tags=headless.
func New(_ *app.App, _ *scs.SessionManager) *Handler { return &Handler{} }

// Router returns a handler that 404s — the dashboard isn't compiled in headless builds.
func (h *Handler) Router() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
}
