package admin

import (
	"net/http"

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

	// Custom 404 handler — render styled error page instead of plain text.
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		tr.RenderNotFound(w, r)
	})

	// Avatar serving — unauthenticated (like static files).
	r.Get("/avatars/preview/{seed}", AvatarPreviewHandler())
	r.Get("/avatars/{id}", AvatarHandler(a))

	// Unauthenticated routes.
	r.Group(func(r chi.Router) {
		r.Use(OAuthLoginReturnMiddleware(sm))
		r.Get("/login", LoginHandler(sm, a.Queries, tr))
		r.Post("/login", LoginHandler(sm, a.Queries, tr))
	})
	r.Post("/logout", LogoutHandler(sm))

	// Token-based account setup (unauthenticated — member uses a link from admin).
	r.With(CSRFMiddleware(sm)).Get("/setup-account/{token}", SetupAccountHandler(sm, a.Queries, tr))
	r.With(CSRFMiddleware(sm)).Post("/setup-account/{token}", SetupAccountHandler(sm, a.Queries, tr))

	// OAuth 2.1 authorize flow (needs session for consent screen).
	r.Get("/oauth/authorize", OAuthAuthorizeHandler(svc, sm, tr))
	r.Post("/oauth/authorize", OAuthAuthorizeSubmitHandler(svc, sm))

	// First-run admin creation (unauthenticated).
	r.Get("/setup", CreateAdminHandler(a.Queries, sm, tr))
	r.Post("/setup", CreateAdminHandler(a.Queries, sm, tr))

	// Programmatic setup API (unauthenticated).
	r.Post("/-/setup", ProgrammaticSetupHandler(a.Queries, sm))

	// Authenticated routes accessible to all roles (admin + member).
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(sm, a.Queries))
		r.Use(CSRFMiddleware(sm))
		r.Use(NavBadgesMiddleware(a.Queries, a.Logger))

		r.Get("/", DashboardHandler(a, svc, tr))
		r.Get("/getting-started", GettingStartedHandler(a, sm, tr))
		r.Post("/getting-started/dismiss", DismissGettingStartedHandler(a, sm))
		r.Post("/getting-started/reopen", ReopenGettingStartedHandler(a, sm))
		r.Get("/insights", InsightsHandler(a, svc, tr))

		r.Route("/connections", func(r chi.Router) {
			r.Get("/", ConnectionsListHandler(a, svc, sm, tr))
			r.Get("/{id}", ConnectionDetailHandler(a, sm, tr))
			// Admin-only connection sub-pages.
			r.With(RequireAdmin(sm)).Get("/new", NewConnectionHandler(a, tr))
			r.With(RequireAdmin(sm)).Get("/import-csv", CSVImportPageHandler(a, tr))
			r.With(RequireAdmin(sm)).Get("/{id}/reauth", ConnectionReauthHandler(a, tr))
		})

		r.Get("/transactions", TransactionListHandler(a, sm, tr, svc))
		r.Get("/transactions/search", TransactionSearchHandler(a, sm, tr, svc))
		r.Get("/transactions/{id}", TransactionDetailHandler(a, sm, tr, svc))
		r.Get("/accounts/{id}", AccountDetailHandler(a, sm, tr, svc))

		// Member account self-service pages.
		r.Get("/my-account", MyAccountHandler(a, sm, tr, svc))
		r.Post("/my-account/password", MyAccountChangePasswordHandler(a, sm))
		r.Post("/my-account/avatar", UploadMyAvatarHandler(a, sm))
		r.Delete("/my-account/avatar", DeleteMyAvatarHandler(a, sm))
		r.Post("/my-account/avatar/regenerate", RegenerateMyAvatarHandler(a, sm))
		r.Post("/my-account/wipe-data", MyAccountWipeDataHandler(a, sm))

		// Password change works for both admin and member sessions.
		r.Post("/settings/password", ChangePasswordHandler(a, sm))
	})

	// Editor+ authenticated routes (HTML pages) — editors can view reviews,
	// access/agents pages, and create (but not revoke) API keys and OAuth clients.
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(sm, a.Queries))
		r.Use(RequireEditor(sm))
		r.Use(CSRFMiddleware(sm))
		r.Use(NavBadgesMiddleware(a.Queries, a.Logger))

		r.Get("/reviews", ReviewsPageHandler(a, sm, tr, svc))

		// Access page (API Keys + OAuth Clients) — editors can view and create.
		r.Get("/access", AccessPageHandler(svc, sm, tr))

		r.Route("/api-keys", func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/access", http.StatusMovedPermanently)
			})
			r.Get("/new", APIKeyNewPageHandler(tr))
			r.Post("/new", APIKeyCreatePageHandler(svc, sm, tr))
			r.Get("/{id}/created", APIKeyCreatedPageHandler(sm, tr))
			// Revoke is admin-only — handled in the admin group below.
		})

		r.Route("/oauth-clients", func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/access", http.StatusMovedPermanently)
			})
			r.Get("/new", OAuthClientNewPageHandler(tr))
			r.Post("/new", OAuthClientCreatePageHandler(svc, sm, tr))
			r.Get("/{id}/created", OAuthClientCreatedPageHandler(sm, tr))
			// Revoke is admin-only — handled in the admin group below.
		})

		// Agents page — editors can view.
		r.Get("/agents", AgentsPageHandler(svc, mcpServer, sm, tr))
		r.Get("/agents/sessions/{id}", SessionDetailHandler(svc, sm, tr))
		r.Get("/agent-wizard/{type}", PromptBuilderHandler(sm, tr))
		r.Get("/agent-wizard/{type}/copy", PromptCopyHandler())
	})

	// Admin-only authenticated routes (HTML pages).
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(sm, a.Queries))
		r.Use(RequireAdmin(sm))
		r.Use(CSRFMiddleware(sm))
		r.Use(NavBadgesMiddleware(a.Queries, a.Logger))

		// Legacy onboarding dismiss route — redirect to new handler.
		r.Post("/onboarding/dismiss", DismissGettingStartedHandler(a, sm))

		r.Route("/users", func(r chi.Router) {
			r.Get("/", UsersListHandler(a, tr))
			r.Get("/new", NewUserHandler(a, tr))
			r.Get("/{id}/edit", EditUserHandler(a, tr))
			r.Get("/{id}/create-login", CreateLoginPageHandler(a, tr))
		})

		// API key and OAuth client revoke — admin only.
		r.Post("/api-keys/{id}/revoke", APIKeyRevokePageHandler(svc, sm))
		r.Post("/oauth-clients/{id}/revoke", OAuthClientRevokePageHandler(svc, sm))

		r.Get("/logs", LogsPageHandler(a, svc, sm, tr))
		r.Get("/sync-logs", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/logs?tab=syncs", http.StatusMovedPermanently)
		})
		r.Get("/sync-logs/{id}", SyncLogDetailHandler(a, sm, tr, svc))
		r.Get("/webhook-events", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/logs?tab=webhooks", http.StatusMovedPermanently)
		})

		r.Get("/account-links", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/connections?tab=links", http.StatusMovedPermanently)
		})
		r.Get("/account-links/{id}", AccountLinkDetailHandler(a, svc, sm, tr))

		r.Get("/reports", ReportsPageHandler(a, svc, sm, tr))
		r.Get("/reports/{id}", ReportDetailHandler(a, svc, sm, tr))
		r.Get("/review-instructions", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/agents?tab=settings", http.StatusMovedPermanently)
		})
		r.Get("/mcp-getting-started", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/agents?tab=guide", http.StatusMovedPermanently)
		})
		r.Get("/agent-wizard", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/agents?tab=wizard", http.StatusMovedPermanently)
		})
		r.Get("/rules", RulesPageHandler(svc, sm, tr, a.Config.Version))
		r.Get("/rules/new", RuleFormPageHandler(svc, sm, tr))
		r.Get("/rules/{id}", RuleDetailPageHandler(svc, sm, tr))
		r.Get("/rules/{id}/edit", RuleFormPageHandler(svc, sm, tr))

		r.Get("/categories", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/rules?tab=categories", http.StatusMovedPermanently)
		})

		r.Route("/mcp-settings", func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/agents?tab=settings", http.StatusMovedPermanently)
			})
			r.Post("/mode", MCPSaveModeHandler(svc, sm))
			r.Post("/tools", MCPSaveToolsHandler(svc, mcpServer, sm))
			r.Post("/instructions", MCPSaveInstructionsHandler(svc, sm))
			r.Post("/review-guidelines", MCPSaveReviewGuidelinesHandler(svc, sm))
			r.Post("/report-format", MCPSaveReportFormatHandler(svc, sm))
		})

		r.Get("/providers", ProvidersGetHandler(a, svc, sm, tr))
		r.Post("/providers/plaid", ProvidersSavePlaidHandler(a, sm))
		r.Post("/providers/teller", ProvidersSaveTellerHandler(a, sm))

		r.Get("/backups", BackupsPageHandler(a, sm, tr))

		r.Get("/settings", SettingsGetHandler(a, sm, tr))
		r.Post("/settings/sync", SettingsSyncPostHandler(a, sm))
		r.Post("/settings/retention", SettingsRetentionPostHandler(a, sm))
	})

	// Admin API (authenticated, JSON responses).
	r.Route("/-", func(r chi.Router) {
		r.Use(RequireAuth(sm, a.Queries))
		r.Use(CSRFMiddleware(sm))

		// Quick search — accessible to all roles.
		r.Get("/search/transactions", QuickSearchTransactionsHandler(svc))

		// Transaction comments — accessible to all roles.
		r.Post("/transactions/{id}/comments", CreateTransactionCommentHandler(a, sm, svc))
		r.Delete("/transactions/{id}/comments/{comment_id}", DeleteTransactionCommentHandler(a, sm, svc))

		// Editor+ API routes (categorization, reviews, access management).
		r.Group(func(r chi.Router) {
			r.Use(RequireEditor(sm))

			// Transaction category override
			r.Post("/transactions/{id}/category", SetTransactionCategoryAdminHandler(svc))
			r.Delete("/transactions/{id}/category", ResetTransactionCategoryAdminHandler(svc))

			// Transaction bulk categorize
			r.Post("/transactions/batch-categorize", BatchSetTransactionCategoryAdminHandler(svc))

			// Review queue (submit/dismiss)
			r.Post("/reviews/{id}/submit", SubmitReviewAdminHandler(a, sm, svc))
			r.Post("/reviews/{id}/dismiss", DismissReviewAdminHandler(a, sm, svc))

			// API keys — editors can list and create (revoke is admin-only below).
			r.Get("/api-keys", ListAPIKeysHandler(svc))
			r.Post("/api-keys", CreateAPIKeyHandler(svc))

			// OAuth clients — editors can list and create (revoke is admin-only below).
			r.Get("/oauth-clients", ListOAuthClientsHandler(svc))
			r.Post("/oauth-clients", CreateOAuthClientHandler(svc))
		})

		// Admin-only API routes.
		r.Group(func(r chi.Router) {
			r.Use(RequireAdmin(sm))

			r.Post("/link-token", LinkTokenHandler(a))
			r.Post("/exchange-token", ExchangeTokenHandler(a))
			r.Post("/connections/{id}/reauth", ConnectionReauthAPIHandler(a))
			r.Post("/connections/{id}/reauth-complete", ConnectionReauthCompleteHandler(a))
			r.Post("/connections/{id}/sync", SyncConnectionHandler(a))
			r.Post("/connections/sync-all", SyncAllConnectionsHandler(a))
			r.Post("/connections/{id}/paused", UpdateConnectionPausedHandler(a, sm))
			r.Post("/connections/{id}/sync-interval", UpdateConnectionSyncIntervalHandler(a, sm))
			r.Delete("/connections/{id}", DeleteConnectionHandler(a, sm))
			r.Post("/accounts/{id}/excluded", UpdateAccountExcludedHandler(a, sm))
			r.Post("/accounts/{id}/display-name", UpdateAccountDisplayNameHandler(a, sm))
			r.Post("/test-provider/{provider}", ProvidersTestHandler(a))
			r.Post("/users", CreateUserHandler(a, sm))
			r.Put("/users/{id}", UpdateUserHandler(a, sm))
			r.Post("/users/{id}/avatar", UploadUserAvatarHandler(a))
			r.Delete("/users/{id}/avatar", DeleteUserAvatarHandler(a))
			r.Post("/users/{id}/avatar/regenerate", RegenerateUserAvatarHandler(a))

			r.Post("/csv/upload", CSVUploadHandler(a, sm))
			r.Post("/csv/preview", CSVPreviewHandler(a, sm))
			r.Post("/csv/import", CSVImportHandler(a, sm, svc))

			// API key + OAuth client revoke/delete — admin only.
			r.Delete("/api-keys/{id}", RevokeAPIKeyHandler(svc))
			r.Delete("/oauth-clients/{id}", RevokeOAuthClientHandler(svc))

			r.Post("/update/dismiss", DismissUpdateHandler(a))
			r.Post("/update", TriggerUpdateHandler(a))

			// Backups
			r.Post("/backups/create", CreateBackupHandler(a, sm))
			r.Get("/backups/{filename}/download", DownloadBackupHandler(a))
			r.Post("/backups/{filename}/restore", RestoreExistingBackupHandler(a, sm))
			r.Post("/backups/{filename}/delete", DeleteBackupHandler(a, sm))
			r.Post("/backups/restore", RestoreBackupHandler(a, sm))
			r.Post("/backups/schedule", BackupScheduleHandler(a, sm))

			// Category CRUD
			r.Post("/categories", CreateCategoryAdminHandler(svc))
			r.Put("/categories/{id}", UpdateCategoryAdminHandler(svc))
			r.Delete("/categories/{id}", DeleteCategoryAdminHandler(svc))
			r.Post("/categories/{id}/merge", MergeCategoryAdminHandler(svc))

			// Category bulk export/import (TSV)
			r.Get("/categories/export-tsv", ExportCategoriesTSVAdminHandler(svc))
			r.Post("/categories/import-tsv", ImportCategoriesTSVAdminHandler(svc))

			// Transaction CSV export
			r.Get("/transactions/export-csv", ExportTransactionsCSVHandler(a, svc))

			// Review queue (admin-only bulk operations)
			r.Post("/reviews/dismiss-all", DismissAllReviewsAdminHandler(a, sm, svc))
			r.Post("/reviews/enqueue", EnqueueReviewAdminHandler(a, sm, svc))
			r.Post("/reviews/enqueue-existing", EnqueueExistingReviewsHandler(a, sm, svc))
			r.Post("/reviews/settings", ReviewSettingsHandler(a, sm))

			// Transaction rules
			r.Post("/rules", CreateRuleAdminHandler(svc, sm))
			r.Put("/rules/{id}", UpdateRuleAdminHandler(svc, sm))
			r.Delete("/rules/{id}", DeleteRuleAdminHandler(svc))
			r.Post("/rules/{id}/toggle", ToggleRuleAdminHandler(svc))
			r.Post("/rules/{id}/apply", ApplyRuleAdminHandler(svc))

			// Account links
			r.Post("/account-links", CreateAccountLinkAdminHandler(svc, sm))
			r.Post("/account-links/{id}/delete", DeleteAccountLinkAdminHandler(svc, sm))
			r.Post("/account-links/{id}/reconcile", ReconcileAccountLinkAdminHandler(svc, sm))
			r.Post("/transaction-matches/{id}/confirm", ConfirmMatchAdminHandler(svc, sm))
			r.Post("/transaction-matches/{id}/reject", RejectMatchAdminHandler(svc, sm))

			// Agent reports
			r.Post("/reports/{id}/read", MarkReportReadAdminHandler(svc))
			r.Post("/reports/read-all", MarkAllReportsReadAdminHandler(svc))

			// Login account management
			r.Get("/members", ListLoginAccountsHandler(svc))
			r.Post("/members", CreateLoginAccountHandler(svc, sm))
			r.Put("/members/{id}/role", UpdateLoginAccountRoleHandler(svc, sm))
			r.Post("/members/{id}/setup-token", RegenerateSetupTokenHandler(svc, sm))
			r.Delete("/members/{id}", DeleteLoginAccountHandler(svc, sm))
			r.Post("/users/{id}/wipe", WipeUserDataHandler(a, sm))
		})
	})

	return r
}
