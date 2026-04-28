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
	r.Get("/setup", CreateAdminHandler(a, sm, tr))
	r.Post("/setup", CreateAdminHandler(a, sm, tr))

	// Programmatic setup API (unauthenticated).
	r.Post("/-/setup", ProgrammaticSetupHandler(a, sm))

	// Authenticated routes accessible to all roles (admin + member).
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(sm, a.Queries))
		r.Use(CSRFMiddleware(sm))
		r.Use(NavBadgesMiddleware(a.Queries, a.Logger))

		r.Get("/", DashboardHandler(a, svc, tr))
		r.Get("/getting-started", GettingStartedHandler(a, sm, tr))
		r.Post("/getting-started/dismiss", DismissGettingStartedHandler(a, sm))
		r.Post("/getting-started/reopen", ReopenGettingStartedHandler(a, sm))

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

		// The review queue is tag-backed — "needs-review" transactions. The
		// `/reviews` alias exists so dashboard CTAs and the `gr` keyboard
		// shortcut point at a stable, meaningful URL.
		r.Get("/reviews", ReviewsAliasHandler)

		// Member account self-service pages — canonical paths under
		// /settings/account/*. Old /my-account/* paths are kept as
		// 301/308 redirects so any straggling external bookmarks or in-
		// flight forms continue to work.
		r.Get("/settings/account", MyAccountHandler(a, sm, tr))
		r.Get("/settings/profile", MyProfileHandler(a, sm, tr))
		r.Post("/settings/account/password", MyAccountChangePasswordHandler(a, sm))
		r.Put("/settings/account/profile", MyAccountUpdateProfileHandler(a, sm))
		r.Post("/settings/account/avatar", UploadMyAvatarHandler(a, sm))
		r.Delete("/settings/account/avatar", DeleteMyAvatarHandler(a, sm))
		r.Post("/settings/account/avatar/regenerate", RegenerateMyAvatarHandler(a, sm))
		r.Post("/settings/account/link-user", LinkAdminToUserHandler(a, sm))
		r.Post("/settings/account/wipe-data", MyAccountWipeDataHandler(a, sm))

		// Legacy /my-account/* redirects.
		r.Get("/my-account", redirectGET("/settings/account"))
		r.Post("/my-account/password", redirectPreserveMethod("/settings/account/password"))
		r.Put("/my-account/profile", redirectPreserveMethod("/settings/account/profile"))
		r.Post("/my-account/avatar", redirectPreserveMethod("/settings/account/avatar"))
		r.Delete("/my-account/avatar", redirectPreserveMethod("/settings/account/avatar"))
		r.Post("/my-account/avatar/regenerate", redirectPreserveMethod("/settings/account/avatar/regenerate"))
		r.Post("/my-account/link-user", redirectPreserveMethod("/settings/account/link-user"))
		r.Post("/my-account/wipe-data", redirectPreserveMethod("/settings/account/wipe-data"))

		// Password change works for both admin and member sessions.
		r.Post("/settings/password", ChangePasswordHandler(a, sm))
	})

	// Editor+ authenticated routes (HTML pages) — editors can view the tag
	// admin, access/agents pages, and create (but not revoke) API keys and
	// OAuth clients.
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(sm, a.Queries))
		r.Use(RequireEditor(sm))
		r.Use(CSRFMiddleware(sm))
		r.Use(NavBadgesMiddleware(a.Queries, a.Logger))

		r.Get("/tags", TagsPageHandler(svc, sm, tr))

		// API Keys + OAuth Clients page (Settings → API Keys tab).
		// Editors can view and create.
		r.Get("/settings/api-keys", AccessPageHandler(svc, sm, tr))

		r.Route("/settings/api-keys", func(r chi.Router) {
			r.Get("/new", APIKeyNewPageHandler(sm, tr))
			r.Post("/new", APIKeyCreatePageHandler(svc, sm, tr))
			r.Get("/{id}/created", APIKeyCreatedPageHandler(sm, tr))
			// Revoke is admin-only — handled in the admin group below.
		})

		r.Route("/settings/oauth-clients", func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/settings/api-keys", http.StatusMovedPermanently)
			})
			r.Get("/new", OAuthClientNewPageHandler(sm, tr))
			r.Post("/new", OAuthClientCreatePageHandler(svc, sm, tr))
			r.Get("/{id}/created", OAuthClientCreatedPageHandler(sm, tr))
			// Revoke is admin-only — handled in the admin group below.
		})

		// Legacy /access, /api-keys, /oauth-clients redirects.
		r.Get("/access", redirectGET("/settings/api-keys"))
		r.Get("/api-keys", redirectGET("/settings/api-keys"))
		r.Get("/api-keys/new", redirectGET("/settings/api-keys/new"))
		r.Post("/api-keys/new", redirectPreserveMethod("/settings/api-keys/new"))
		r.Get("/api-keys/{id}/created", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/settings/api-keys/"+chi.URLParam(r, "id")+"/created", http.StatusMovedPermanently)
		})
		r.Get("/oauth-clients", redirectGET("/settings/api-keys"))
		r.Get("/oauth-clients/new", redirectGET("/settings/oauth-clients/new"))
		r.Post("/oauth-clients/new", redirectPreserveMethod("/settings/oauth-clients/new"))
		r.Get("/oauth-clients/{id}/created", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/settings/oauth-clients/"+chi.URLParam(r, "id")+"/created", http.StatusMovedPermanently)
		})

		// Agent Prompts page (formerly /agents) — editors can view.
		// Per-session detail lives under /logs/sessions/{id} since
		// session data is hosted on the Logs page's Sessions tab;
		// it's registered here under the editor+ scope to preserve the
		// previous /agents/sessions/{id} permission level.
		r.Get("/agent-prompts", AgentsPageHandler(svc, sm, tr))
		r.Get("/agent-prompts/builder/{type}", PromptBuilderHandler(sm, tr))
		r.Get("/agent-prompts/builder/{type}/copy", PromptCopyHandler())
		r.Get("/logs/sessions/{id}", SessionDetailHandler(svc, sm, tr))

		// Legacy /agents, /agent-wizard, and /activity/sessions redirects.
		r.Get("/agents", redirectGET("/agent-prompts"))
		r.Get("/agents/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/logs/sessions/"+chi.URLParam(r, "id"), http.StatusMovedPermanently)
		})
		r.Get("/activity/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/logs/sessions/"+chi.URLParam(r, "id"), http.StatusMovedPermanently)
		})
		r.Get("/agent-wizard/{type}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/agent-prompts/builder/"+chi.URLParam(r, "type"), http.StatusMovedPermanently)
		})
		r.Get("/agent-wizard/{type}/copy", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/agent-prompts/builder/"+chi.URLParam(r, "type")+"/copy", http.StatusMovedPermanently)
		})
	})

	// Admin-only authenticated routes (HTML pages).
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(sm, a.Queries))
		r.Use(RequireAdmin(sm))
		r.Use(CSRFMiddleware(sm))
		r.Use(NavBadgesMiddleware(a.Queries, a.Logger))

		// Legacy onboarding dismiss route — redirect to new handler.
		r.Post("/onboarding/dismiss", DismissGettingStartedHandler(a, sm))

		r.Route("/settings/household", func(r chi.Router) {
			r.Get("/", UsersListHandler(a, sm, tr))
			r.Get("/new", NewUserHandler(a, sm, tr))
			r.Get("/{id}/edit", EditUserHandler(a, sm, tr))
			r.Get("/{id}/create-login", CreateLoginPageHandler(a, tr))
		})

		// Legacy /users redirects.
		r.Get("/users", redirectGET("/settings/household"))
		r.Get("/users/new", redirectGET("/settings/household/new"))
		r.Get("/users/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/settings/household/"+chi.URLParam(r, "id")+"/edit", http.StatusMovedPermanently)
		})
		r.Get("/users/{id}/create-login", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/settings/household/"+chi.URLParam(r, "id")+"/create-login", http.StatusMovedPermanently)
		})

		// API key and OAuth client revoke — admin only.
		r.Post("/settings/api-keys/{id}/revoke", APIKeyRevokePageHandler(svc, sm))
		r.Post("/settings/oauth-clients/{id}/revoke", OAuthClientRevokePageHandler(svc, sm))
		r.Post("/api-keys/{id}/revoke", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/settings/api-keys/"+chi.URLParam(r, "id")+"/revoke", http.StatusPermanentRedirect)
		})
		r.Post("/oauth-clients/{id}/revoke", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/settings/oauth-clients/"+chi.URLParam(r, "id")+"/revoke", http.StatusPermanentRedirect)
		})

		// Logs page — sync history, webhook events, agent sessions tabs.
		// The per-sync-log detail page lives under /logs/sync-logs/{id};
		// the per-session detail page lives under /logs/sessions/{id}
		// (since session data is on the Logs page's Sessions tab).
		r.Get("/logs", LogsPageHandler(a, svc, sm, tr))
		r.Get("/logs/sync-logs/{id}", SyncLogDetailHandler(a, sm, tr, svc))

		// Legacy /activity, /sync-logs, and /webhook-events redirects.
		r.Get("/activity", redirectGET("/logs"))
		r.Get("/sync-logs", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/logs?tab=syncs", http.StatusMovedPermanently)
		})
		r.Get("/sync-logs/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/logs/sync-logs/"+chi.URLParam(r, "id"), http.StatusMovedPermanently)
		})
		r.Get("/activity/sync-logs/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/logs/sync-logs/"+chi.URLParam(r, "id"), http.StatusMovedPermanently)
		})
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
			http.Redirect(w, r, "/settings/mcp", http.StatusMovedPermanently)
		})

		// MCP tab inside the Settings shell — MCP server instructions,
		// review guidelines, report format, and per-tool toggles. POST
		// targets remain at /-/mcp-settings/* (registered below).
		r.Get("/settings/mcp", AgentsSettingsHandler(svc, mcpServer, sm, tr))
		r.Get("/agents-settings", redirectGET("/settings/mcp"))
		r.Get("/mcp-getting-started", redirectGET("/agent-prompts"))
		r.Get("/agent-wizard", redirectGET("/agent-prompts"))
		r.Get("/rules", RulesPageHandler(svc, sm, tr, a.Config.Version))
		r.Get("/rules/new", RuleFormPageHandler(svc, sm, tr))
		r.Get("/rules/{id}", RuleDetailPageHandler(svc, sm, tr))
		r.Get("/rules/{id}/edit", RuleFormPageHandler(svc, sm, tr))

		// Tag create/edit pages — the /tags list itself is accessible to editors,
		// but the admin-only JSON endpoints back the form submit. Keep the
		// modal-era POST /-/tags endpoint untouched so the transaction tag
		// picker's quick-create path keeps working.
		r.Get("/tags/new", TagNewPageHandler(sm, tr))
		r.Get("/tags/{id}/edit", TagEditPageHandler(svc, sm, tr))

		r.Get("/categories", CategoriesPageHandler(svc, sm, tr))
		r.Get("/categories/new", CategoryNewPageHandler(svc, sm, tr))
		r.Get("/categories/{id}/edit", CategoryEditPageHandler(svc, sm, tr))

		r.Route("/mcp-settings", func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/settings/mcp", http.StatusMovedPermanently)
			})
			r.Post("/mode", MCPSaveModeHandler(svc, sm))
			r.Post("/tools", MCPSaveToolsHandler(svc, mcpServer, sm))
			r.Post("/instructions", MCPSaveInstructionsHandler(svc, sm))
			r.Post("/review-guidelines", MCPSaveReviewGuidelinesHandler(svc, sm))
			r.Post("/report-format", MCPSaveReportFormatHandler(svc, sm))
		})

		r.Get("/settings/providers", ProvidersGetHandler(a, svc, sm, tr))
		r.Post("/settings/providers/plaid", ProvidersSavePlaidHandler(a, sm))
		r.Post("/settings/providers/teller", ProvidersSaveTellerHandler(a, sm))
		r.Get("/providers", redirectGET("/settings/providers"))
		r.Post("/providers/plaid", redirectPreserveMethod("/settings/providers/plaid"))
		r.Post("/providers/teller", redirectPreserveMethod("/settings/providers/teller"))

		r.Get("/settings/backups", BackupsPageHandler(a, sm, tr))
		r.Get("/backups", redirectGET("/settings/backups"))

		r.Get("/settings", SettingsGetHandler(a, sm, tr))
		r.Get("/settings/sync", SettingsGetHandler(a, sm, tr))
		r.Get("/settings/security", SecuritySettingsHandler(a, sm, tr))
		r.Get("/settings/system", SystemSettingsHandler(a, sm, tr))
		r.Get("/settings/help", HelpSettingsHandler(a, sm, tr))
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

		// Activity-timeline partial render — powers the optimistic-update
		// flow on the detail page. Accessible to all roles (read-only).
		r.Get("/transactions/{id}/timeline/rows", TimelineRowsHandler(a, sm, svc))

		// Editor+ API routes (categorization, tagging, access management).
		r.Group(func(r chi.Router) {
			r.Use(RequireEditor(sm))

			// Transaction category override
			r.Post("/transactions/{id}/category", SetTransactionCategoryAdminHandler(svc, sm))
			r.Delete("/transactions/{id}/category", ResetTransactionCategoryAdminHandler(svc, sm))
			r.Patch("/transactions/{id}/category-override", SetCategoryOverrideAdminHandler(svc, sm))

			// Transaction bulk categorize
			r.Post("/transactions/batch-categorize", BatchSetTransactionCategoryAdminHandler(svc, sm))

			// Batch compound update (bulk actions on transactions list).
			r.Post("/transactions/batch-update", BulkUpdateTransactionsAdminHandler(a, sm, svc))

			// Single-transaction tag operations (used by detail page chip UI).
			r.Post("/transactions/{id}/tags", AddTransactionTagAdminHandler(a, sm, svc))
			r.Delete("/transactions/{id}/tags/{slug}", RemoveTransactionTagAdminHandler(a, sm, svc))

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
			r.Get("/connections/{id}/sync-status", SyncConnectionStatusHandler(a))
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

			// Tag CRUD (admin-only).
			r.Post("/tags", CreateTagAdminHandler(svc))
			r.Put("/tags/{id}", UpdateTagAdminHandler(svc))
			r.Delete("/tags/{id}", DeleteTagAdminHandler(svc))

			// Transaction rules
			r.Post("/rules", CreateRuleAdminHandler(svc, sm))
			r.Post("/rules/preview", PreviewRuleAdminHandler(svc))
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
			r.Post("/reports/{id}/unread", MarkReportUnreadAdminHandler(svc))
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
