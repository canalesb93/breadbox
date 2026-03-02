package api

import (
	"net/http"

	"breadbox/internal/admin"
	"breadbox/internal/app"
	mw "breadbox/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter creates a chi router with middleware and all route mounts.
func NewRouter(a *app.App, version string) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(mw.Logging(a.Logger))
	r.Use(middleware.Recoverer)

	r.Get("/health", HealthHandler(version))

	// Admin dashboard: session manager + template renderer + admin router.
	isSecure := a.Config.Environment == "production" || a.Config.Environment == "docker"
	sm := admin.NewSessionManager(a.DB, isSecure)
	tr, err := admin.NewTemplateRenderer()
	if err != nil {
		a.Logger.Error("failed to initialize template renderer", "error", err)
	} else {
		adminRouter := admin.NewAdminRouter(a, sm, tr)
		r.Mount("/", adminRouter)
	}

	return r
}
