package api

import (
	"net/http"

	"breadbox/internal/admin"
	"breadbox/internal/app"
	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

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

	// REST API v1 — API key authenticated.
	svc := service.New(a.Queries, a.DB, a.SyncEngine, a.Logger)
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))
		r.Get("/accounts", ListAccountsHandler(svc))
		r.Get("/accounts/{id}", GetAccountHandler(svc))
		r.Get("/transactions", ListTransactionsHandler(svc))
		r.Get("/transactions/count", CountTransactionsHandler(svc))
		r.Get("/transactions/{id}", GetTransactionHandler(svc))
		r.Get("/users", ListUsersHandler(svc))
		r.Get("/connections", ListConnectionsHandler(svc))
		r.Get("/connections/{id}/status", GetConnectionStatusHandler(svc))
		r.Post("/sync", TriggerSyncHandler(svc))
	})

	// Admin dashboard: session manager + template renderer + admin router.
	isSecure := a.Config.Environment == "production" || a.Config.Environment == "docker"
	sm := admin.NewSessionManager(a.DB, isSecure)
	tr, err := admin.NewTemplateRenderer()
	if err != nil {
		a.Logger.Error("failed to initialize template renderer", "error", err)
	} else {
		adminRouter := admin.NewAdminRouter(a, sm, tr, svc)
		r.Mount("/", adminRouter)
	}

	return r
}
