package api

import (
	"net/http"

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

	return r
}
