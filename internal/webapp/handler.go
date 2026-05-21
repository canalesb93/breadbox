//go:build !headless && !lite

package webapp

import (
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"breadbox/internal/app"
)

// Handler is the /app subrouter. It holds the shared App (service layer, queries,
// config) and the session manager shared with v1 admin and the v2 SPA.
type Handler struct {
	app *app.App
	sm  *scs.SessionManager
}

// New builds the webapp handler. The session manager must be the same instance the
// rest of the dashboard uses so a single login cookie works across /, /v2, and /app.
func New(a *app.App, sm *scs.SessionManager) *Handler {
	return &Handler{app: a, sm: sm}
}

// Router returns the chi subrouter mounted at /app. Every page is a real document;
// there is no client router. Navigation, history, scroll, and bfcache belong to the browser.
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()

	// Static assets first — no session, so they stay cacheable.
	r.Handle("/static/*", staticHandler())

	// Session is loaded for all dynamic routes (pages render with the current user).
	r.Group(func(r chi.Router) {
		r.Use(h.sm.LoadAndSave)

		// Public (pre-auth) routes.
		r.Get("/login", h.loginPage)
		r.Post("/login", h.requireSameOrigin(h.loginSubmit))

		// Authenticated routes. The gate is a real server-side 302 to /app/login —
		// so the SPA's "401 redirect trap" class of bug cannot exist here.
		r.Group(func(r chi.Router) {
			r.Use(h.requireAuth)

			r.Get("/", h.home)
			r.Post("/logout", h.requireSameOrigin(h.logout))

			r.Get("/accounts", h.accountsList)
			r.Get("/accounts/{id}", h.accountDetail)
		})
	})

	return r
}
