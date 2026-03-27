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
	r.Group(func(r chi.Router) {
		r.Use(OAuthLoginReturnMiddleware(sm))
		r.Get("/login", LoginHandler(sm, a.Queries, tr))
		r.Post("/login", LoginHandler(sm, a.Queries, tr))
	})
	r.Post("/logout", LogoutHandler(sm))

	// OAuth 2.1 authorize flow (needs session for consent screen).
	r.Get("/oauth/authorize", OAuthAuthorizeHandler(svc, sm, tr))
	r.Post("/oauth/authorize", OAuthAuthorizeSubmitHandler(svc, sm))

	// First-run admin creation (unauthenticated).
	r.Get("/setup", CreateAdminHandler(a.Queries, sm, tr))
	r.Post("/setup", CreateAdminHandler(a.Queries, sm, tr))

	// Programmatic setup API (unauthenticated).
	r.Post("/-/setup", ProgrammaticSetupHandler(a.Queries, sm))

	// Authenticated admin routes (HTML pages).
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(sm))
		r.Use(CSRFMiddleware(sm))
		r.Use(NavBadgesMiddleware(a.Queries, a.Logger))

		r.Get("/", DashboardHandler(a, svc, tr))
		r.Get("/insights", InsightsHandler(a, svc, tr))
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

		r.Route("/oauth-clients", func(r chi.Router) {
			r.Get("/", OAuthClientsListPageHandler(svc, sm, tr))
			r.Get("/new", OAuthClientNewPageHandler(tr))
			r.Post("/new", OAuthClientCreatePageHandler(svc, sm, tr))
			r.Get("/{id}/created", OAuthClientCreatedPageHandler(sm, tr))
			r.Post("/{id}/revoke", OAuthClientRevokePageHandler(svc, sm))
		})

		r.Get("/transactions", TransactionListHandler(a, sm, tr, svc))
		r.Get("/transactions/{id}", TransactionDetailHandler(a, sm, tr, svc))
		r.Get("/accounts/{id}", AccountDetailHandler(a, sm, tr, svc))
		r.Get("/sync-logs", SyncLogsHandler(a, sm, tr, svc))

		r.Get("/account-links", AccountLinksPageHandler(a, svc, sm, tr))
		r.Get("/account-links/{id}", AccountLinkDetailHandler(a, svc, sm, tr))

		r.Get("/reports", ReportsPageHandler(a, svc, sm, tr))
		r.Get("/reviews", ReviewsPageHandler(a, sm, tr, svc))
		r.Get("/review-instructions", ReviewInstructionsPageHandler(sm, tr))
		r.Get("/rules", RulesPageHandler(svc, sm, tr, a.Config.Version))

		r.Get("/categories", CategoriesPageHandler(svc, sm, tr))
		r.Get("/categories/mappings", MappingsPageHandler(svc, sm, tr))

		r.Route("/mcp-settings", func(r chi.Router) {
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
	r.Route("/-", func(r chi.Router) {
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

		r.Get("/oauth-clients", ListOAuthClientsHandler(svc))
		r.Post("/oauth-clients", CreateOAuthClientHandler(svc))
		r.Delete("/oauth-clients/{id}", RevokeOAuthClientHandler(svc))

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

		// Transaction CSV export
		r.Get("/transactions/export-csv", ExportTransactionsCSVHandler(a, svc))

		// Transaction bulk categorize
		r.Post("/transactions/batch-categorize", BatchSetTransactionCategoryAdminHandler(svc))

		// Transaction category override
		r.Post("/transactions/{id}/category", SetTransactionCategoryAdminHandler(svc))
		r.Delete("/transactions/{id}/category", ResetTransactionCategoryAdminHandler(svc))

		// Transaction comments
		r.Post("/transactions/{id}/comments", CreateTransactionCommentHandler(a, sm, svc))
		r.Delete("/transactions/{id}/comments/{comment_id}", DeleteTransactionCommentHandler(a, sm, svc))

		// Review queue
		r.Post("/reviews/{id}/submit", SubmitReviewAdminHandler(a, sm, svc))
		r.Post("/reviews/{id}/dismiss", DismissReviewAdminHandler(a, sm, svc))
		r.Post("/reviews/dismiss-all", DismissAllReviewsAdminHandler(a, sm, svc))
		r.Post("/reviews/enqueue", EnqueueReviewAdminHandler(a, sm, svc))
		r.Post("/reviews/settings", ReviewSettingsHandler(a, sm))

		// Transaction rules
		r.Post("/rules", CreateRuleAdminHandler(svc, sm))
		r.Put("/rules/{id}", UpdateRuleAdminHandler(svc, sm))
		r.Delete("/rules/{id}", DeleteRuleAdminHandler(svc))
		r.Post("/rules/{id}/toggle", ToggleRuleAdminHandler(svc))

		// Account links
		r.Post("/account-links", CreateAccountLinkAdminHandler(svc, sm))
		r.Post("/account-links/{id}/delete", DeleteAccountLinkAdminHandler(svc, sm))
		r.Post("/account-links/{id}/reconcile", ReconcileAccountLinkAdminHandler(svc, sm))
		r.Post("/transaction-matches/{id}/confirm", ConfirmMatchAdminHandler(svc, sm))
		r.Post("/transaction-matches/{id}/reject", RejectMatchAdminHandler(svc, sm))

		// Agent reports
		r.Post("/reports/{id}/read", MarkReportReadAdminHandler(svc))
		r.Post("/reports/read-all", MarkAllReportsReadAdminHandler(svc))
	})

	return r
}
