package admin

import (
	"breadbox/internal/app"
	breadboxmcp "breadbox/internal/mcp"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// NewAdminRouter creates the chi.Router for all admin dashboard routes.
// It includes unauthenticated routes (login, setup) and authenticated routes
// (dashboard, connections, users, admin API).
func NewAdminRouter(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service, mcpServer *breadboxmcp.MCPServer) chi.Router {
	r := chi.NewRouter()

	// Session middleware wraps everything so session data is available.
	r.Use(sm.LoadAndSave)

	// Setup detection — redirect to wizard if no admin account exists.
	r.Use(SetupDetection(a.Queries))

	// Unauthenticated routes.
	r.Get("/login", LoginHandler(sm, a.Queries, tr))
	r.Post("/login", LoginHandler(sm, a.Queries, tr))
	r.Post("/logout", LogoutHandler(sm))

	// First-run admin creation (unauthenticated).
	r.Get("/admin/setup", CreateAdminHandler(a.Queries, sm, tr))
	r.Post("/admin/setup", CreateAdminHandler(a.Queries, sm, tr))

	// Programmatic setup API (unauthenticated).
	r.Post("/admin/api/setup", ProgrammaticSetupHandler(a.Queries, sm))

	// Authenticated admin routes (HTML pages).
	r.Route("/admin", func(r chi.Router) {
		r.Use(RequireAuth(sm))
		r.Use(CSRFMiddleware(sm))

		r.Get("/", DashboardHandler(a, tr))
		r.Post("/onboarding/dismiss", DismissOnboardingHandler(a))

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
		r.Get("/transactions/{id}", TransactionDetailHandler(a, sm, tr, svc))
		r.Get("/accounts/{id}", AccountDetailHandler(a, sm, tr, svc))
		r.Get("/sync-logs", SyncLogsHandler(a, sm, tr, svc))

		r.Get("/reviews", ReviewsPageHandler(a, sm, tr, svc))
		r.Get("/reviews/settings", ReviewSettingsPageHandler(a, sm, tr, svc))
		r.Post("/reviews/settings/instructions", ReviewInstructionsSaveHandler(a, sm, svc))
		r.Post("/reviews/settings/webhook", ReviewWebhookSaveHandler(a, sm, svc))

		r.Get("/categories", CategoriesPageHandler(svc, sm, tr))
		r.Get("/categories/mappings", MappingsPageHandler(svc, sm, tr))

		r.Route("/mcp", func(r chi.Router) {
			r.Get("/", MCPSettingsGetHandler(svc, mcpServer, sm, tr))
			r.Post("/mode", MCPSaveModeHandler(svc, sm))
			r.Post("/tools", MCPSaveToolsHandler(svc, mcpServer, sm))
			r.Post("/instructions", MCPSaveInstructionsHandler(svc, sm))
		})

		r.Get("/providers", ProvidersGetHandler(a, sm, tr))
		r.Post("/providers/plaid", ProvidersSavePlaidHandler(a, sm))
		r.Post("/providers/teller", ProvidersSaveTellerHandler(a, sm))

		r.Get("/settings", SettingsGetHandler(a, sm, tr))
		r.Post("/settings/sync", SettingsSyncPostHandler(a, sm))
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
		r.Post("/connections/{id}/paused", UpdateConnectionPausedHandler(a, sm))
		r.Post("/connections/{id}/sync-interval", UpdateConnectionSyncIntervalHandler(a, sm))
		r.Delete("/connections/{id}", DeleteConnectionHandler(a, sm))
		r.Post("/accounts/{id}/excluded", UpdateAccountExcludedHandler(a, sm))
		r.Post("/accounts/{id}/display-name", UpdateAccountDisplayNameHandler(a, sm))
		r.Post("/test-provider/{provider}", ProvidersTestHandler(a))
		r.Post("/users", CreateUserHandler(a, sm))
		r.Put("/users/{id}", UpdateUserHandler(a, sm))

		r.Post("/csv/upload", CSVUploadHandler(a, sm))
		r.Post("/csv/preview", CSVPreviewHandler(a, sm))
		r.Post("/csv/import", CSVImportHandler(a, sm, svc))

		r.Get("/api-keys", ListAPIKeysHandler(svc))
		r.Post("/api-keys", CreateAPIKeyHandler(svc))
		r.Delete("/api-keys/{id}", RevokeAPIKeyHandler(svc))

		r.Post("/update/dismiss", DismissUpdateHandler(a))
		r.Post("/update", TriggerUpdateHandler(a))

		// Category CRUD
		r.Post("/categories", CreateCategoryAdminHandler(svc))
		r.Put("/categories/{id}", UpdateCategoryAdminHandler(svc))
		r.Delete("/categories/{id}", DeleteCategoryAdminHandler(svc))
		r.Post("/categories/{id}/merge", MergeCategoryAdminHandler(svc))

		// Category bulk export/import (TSV)
		r.Get("/categories/export-tsv", ExportCategoriesTSVAdminHandler(svc))
		r.Post("/categories/import-tsv", ImportCategoriesTSVAdminHandler(svc))

		// Category mapping CRUD
		r.Post("/category-mappings", CreateMappingAdminHandler(svc))
		r.Put("/category-mappings/{id}", UpdateMappingAdminHandler(svc))
		r.Delete("/category-mappings/{id}", DeleteMappingAdminHandler(svc))
		r.Put("/category-mappings/bulk", BulkUpsertMappingsAdminHandler(svc))
		r.Get("/category-mappings/export", ExportMappingsAdminHandler(svc))
		r.Get("/category-mappings/export-tsv", ExportMappingsTSVAdminHandler(svc))
		r.Post("/category-mappings/import-tsv", ImportMappingsTSVAdminHandler(svc))

		// Transaction category override
		r.Post("/transactions/{id}/category", SetTransactionCategoryAdminHandler(svc))
		r.Delete("/transactions/{id}/category", ResetTransactionCategoryAdminHandler(svc))

		// Transaction comments
		r.Post("/transactions/{id}/comments", CreateTransactionCommentHandler(a, sm, svc))
		r.Delete("/transactions/{id}/comments/{comment_id}", DeleteTransactionCommentHandler(a, sm, svc))

		// Review queue
		r.Post("/reviews/{id}/submit", SubmitReviewAdminHandler(a, sm, svc))
		r.Post("/reviews/{id}/dismiss", DismissReviewAdminHandler(a, sm, svc))
		r.Post("/reviews/enqueue", EnqueueReviewAdminHandler(a, sm, svc))
		r.Post("/reviews/settings", ReviewSettingsHandler(a, sm))
		r.Post("/review-webhooks/test", ReviewWebhookTestHandler(a, svc))
	})

	return r
}
