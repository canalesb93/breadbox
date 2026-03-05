package admin

import (
	"breadbox/internal/app"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// NewAdminRouter creates the chi.Router for all admin dashboard routes.
// It includes unauthenticated routes (login, setup) and authenticated routes
// (dashboard, connections, users, admin API).
func NewAdminRouter(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) chi.Router {
	r := chi.NewRouter()

	// Session middleware wraps everything so session data is available.
	r.Use(sm.LoadAndSave)

	// Setup detection — redirect to wizard if no admin account exists.
	r.Use(SetupDetection(a.Queries))

	// Unauthenticated routes.
	r.Get("/login", LoginHandler(sm, a.Queries, tr))
	r.Post("/login", LoginHandler(sm, a.Queries, tr))
	r.Post("/logout", LogoutHandler(sm))

	// Setup wizard (unauthenticated).
	r.Route("/admin/setup", func(r chi.Router) {
		r.Get("/step/1", SetupStep1Handler(a.Queries, tr))
		r.Post("/step/1", SetupStep1Handler(a.Queries, tr))
		r.Get("/step/2", SetupStep2Handler(a.Queries, tr))
		r.Post("/step/2", SetupStep2Handler(a.Queries, tr))
		r.Get("/step/3", SetupStep3Handler(a.Queries, tr))
		r.Post("/step/3", SetupStep3Handler(a.Queries, tr))
		r.Get("/step/4", SetupStep4Handler(a.Queries, tr))
		r.Post("/step/4", SetupStep4Handler(a.Queries, tr))
		r.Get("/step/5", SetupStep5Handler(a.Queries, tr))
		r.Post("/step/5", SetupStep5Handler(a.Queries, tr))
		r.Get("/step/6", SetupStep6Handler(a.Queries, tr))
		r.Post("/step/6", SetupStep6Handler(a.Queries, tr))
	})

	// Setup API (unauthenticated).
	r.Get("/admin/api/setup/status", SetupStatusHandler(a.Queries))
	r.Post("/admin/api/setup", ProgrammaticSetupHandler(a.Queries, sm))

	// Authenticated admin routes (HTML pages).
	r.Route("/admin", func(r chi.Router) {
		r.Use(RequireAuth(sm))
		r.Use(CSRFMiddleware(sm))

		r.Get("/", DashboardHandler(a, tr))

		r.Route("/connections", func(r chi.Router) {
			r.Get("/", ConnectionsListHandler(a, tr))
			r.Get("/new", NewConnectionHandler(a, tr))
			r.Get("/import-csv", CSVImportPageHandler(a, tr))
			r.Get("/{id}", ConnectionDetailHandler(a, tr))
			r.Get("/{id}/reauth", ConnectionReauthHandler(a, tr))
		})

		r.Route("/users", func(r chi.Router) {
			r.Get("/", UsersListHandler(a, tr))
			r.Get("/new", NewUserHandler(a, tr))
			r.Get("/{id}/edit", EditUserHandler(a, tr))
		})

		r.Route("/api-keys", func(r chi.Router) {
			r.Get("/", APIKeysListPageHandler(svc, sm, tr))
			r.Get("/new", APIKeyNewPageHandler(tr))
			r.Post("/new", APIKeyCreatePageHandler(svc, sm, tr))
			r.Get("/{id}/created", APIKeyCreatedPageHandler(sm, tr))
			r.Post("/{id}/revoke", APIKeyRevokePageHandler(svc, sm))
		})

		r.Get("/transactions", TransactionListHandler(a, sm, tr, svc))
		r.Get("/accounts/{id}", AccountDetailHandler(a, sm, tr, svc))
		r.Get("/sync-logs", SyncLogsHandler(a, sm, tr, svc))
		r.Get("/settings", SettingsGetHandler(a, sm, tr))
		r.Post("/settings", SettingsPostHandler(a, sm, tr, svc))
		r.Post("/settings/password", ChangePasswordHandler(a, sm))
	})

	// Admin API (authenticated, JSON responses).
	r.Route("/admin/api", func(r chi.Router) {
		r.Use(RequireAuth(sm))

		r.Post("/link-token", LinkTokenHandler(a))
		r.Post("/exchange-token", ExchangeTokenHandler(a))
		r.Post("/connections/{id}/reauth", ConnectionReauthAPIHandler(a))
		r.Post("/connections/{id}/reauth-complete", ConnectionReauthCompleteHandler(a))
		r.Post("/connections/{id}/sync", SyncConnectionHandler(a))
		r.Post("/connections/{id}/paused", UpdateConnectionPausedHandler(a))
		r.Post("/connections/{id}/sync-interval", UpdateConnectionSyncIntervalHandler(a))
		r.Delete("/connections/{id}", DeleteConnectionHandler(a))
		r.Post("/accounts/{id}/excluded", UpdateAccountExcludedHandler(a))
		r.Post("/accounts/{id}/display-name", UpdateAccountDisplayNameHandler(a))
		r.Post("/test-provider/{provider}", TestProviderHandler(a))
		r.Post("/users", CreateUserHandler(a))
		r.Put("/users/{id}", UpdateUserHandler(a))

		r.Post("/csv/upload", CSVUploadHandler(a, sm))
		r.Post("/csv/preview", CSVPreviewHandler(a, sm))
		r.Post("/csv/import", CSVImportHandler(a, sm, svc))

		r.Get("/api-keys", ListAPIKeysHandler(svc))
		r.Post("/api-keys", CreateAPIKeyHandler(svc))
		r.Delete("/api-keys/{id}", RevokeAPIKeyHandler(svc))
	})

	return r
}
