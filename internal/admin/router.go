//go:build !headless && !lite

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

	// Unauthenticated routes. devLogin gates the one-tap quick-login button
	// on the form: shown only for ENVIRONMENT=local, never docker/prod.
	devLogin := a.Config.Environment == "local"
	r.Group(func(r chi.Router) {
		r.Use(OAuthLoginReturnMiddleware(sm))
		r.Get("/login", LoginHandler(sm, a.Queries, tr, devLogin))
		r.Post("/login", LoginHandler(sm, a.Queries, tr, devLogin))
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
		r.Use(DevModeMiddleware(a.Queries))

		r.Get("/", FeedHandler(a, svc, tr))
		// Old `/feed` URL retired — feed is now the root. Redirect any
		// straggling external links (bookmarks, in-app history) to `/`.
		r.Get("/feed", redirectGET("/"))
		r.Get("/getting-started", GettingStartedHandler(a, sm, tr))
		r.Post("/getting-started/dismiss", DismissGettingStartedHandler(a, sm))
		r.Post("/getting-started/reopen", ReopenGettingStartedHandler(a, sm))

		// One-time encryption-key reveal — admin-only, redirects away
		// once acknowledged. Recovery after that is via .env or
		// `breadbox reveal-key`.
		r.With(RequireAdmin(sm)).Get("/setup/save-key", SaveKeyHandler(a, sm, tr))
		r.With(RequireAdmin(sm)).Post("/setup/save-key", SaveKeyHandler(a, sm, tr))

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
		r.Get("/accounts", AccountsListPageHandler(a, svc, sm, tr))
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
		// Profile was its own tab in the legacy SettingsLayout; it has
		// since been merged into the Account tab (single identity page
		// covering avatar + name + email + password + danger zone).
		// Permanent redirect so any straggling bookmarks land on the new
		// canonical URL.
		r.Get("/settings/profile", redirectGET("/settings/account"))
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

		// Recurring — recurring-series list + detail. Editor scope: the
		// candidate-confirmation surface is the single human-adjudication point
		// for detected series. Captures all recurring charges (subscriptions,
		// bills, loans) — "subscription" is one type, not the umbrella.
		r.Get("/recurring", SubscriptionsListPageHandler(a, svc, sm, tr))
		// Create-from-scratch (static segment registered before the {id} param).
		r.Get("/recurring/new", NewRecurringSeriesPageHandler(a, svc, sm, tr))
		r.Post("/recurring/new", CreateRecurringSeriesHandler(a, svc, sm, tr))
		r.Get("/recurring/{id}", SubscriptionDetailHandler(a, sm, tr, svc))
		// Back-compat redirects from the former "Subscriptions" route (302 so a
		// later change isn't browser-cached).
		r.Get("/subscriptions", func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "/recurring", http.StatusFound)
		})
		r.Get("/subscriptions/{id}", func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "/recurring/"+chi.URLParam(req, "id"), http.StatusFound)
		})

		// Design-system sandbox. /design is the full gallery; /design/c/{slug}
		// renders a single component family in isolation for focused screenshots.
		// Editor scope: developer tooling, not user-facing.
		r.Get("/design", DesignGalleryHandler(sm, tr))
		r.Get("/design/c/{slug}", DesignComponentHandler(sm, tr))

		// API Keys + OAuth Clients page (Settings → API Keys tab).
		// Editors can view and create. Creation is one-click: a POST mints
		// the secret and 303-redirects back to the tab, which reveals the
		// plaintext inline (no /new form or /created subpage).
		r.Get("/settings/api-keys", AccessPageHandler(svc, sm, tr))
		r.Post("/settings/api-keys/new", APIKeyCreatePageHandler(svc, sm, tr))

		r.Get("/settings/oauth-clients", redirectGET("/settings/api-keys"))
		r.Post("/settings/oauth-clients/new", OAuthClientCreatePageHandler(svc, sm, tr))

		// Legacy /access, /api-keys, /oauth-clients redirects. The old GET
		// /new + /{id}/created subpages are gone — those URLs now 301 to the
		// list, where create + reveal happen inline.
		r.Get("/access", redirectGET("/settings/api-keys"))
		r.Get("/api-keys", redirectGET("/settings/api-keys"))
		r.Get("/api-keys/new", redirectGET("/settings/api-keys"))
		r.Post("/api-keys/new", redirectPreserveMethod("/settings/api-keys/new"))
		r.Get("/api-keys/{id}/created", redirectGET("/settings/api-keys"))
		r.Get("/settings/api-keys/new", redirectGET("/settings/api-keys"))
		r.Get("/settings/api-keys/{id}/created", redirectGET("/settings/api-keys"))
		r.Get("/oauth-clients", redirectGET("/settings/api-keys"))
		r.Get("/oauth-clients/new", redirectGET("/settings/api-keys"))
		r.Post("/oauth-clients/new", redirectPreserveMethod("/settings/oauth-clients/new"))
		r.Get("/oauth-clients/{id}/created", redirectGET("/settings/api-keys"))
		r.Get("/settings/oauth-clients/new", redirectGET("/settings/api-keys"))
		r.Get("/settings/oauth-clients/{id}/created", redirectGET("/settings/api-keys"))

		// Legacy /agents → Workflows redirects + hand-authored agent form
		// pages (kept reachable by direct URL until the custom-workflow
		// builder lands).
		r.Get("/agents", redirectGET("/workflows/runs"))
		r.Get("/agents/definitions", redirectGET("/workflows"))
		r.Get("/workflows", WorkflowsGalleryPageHandler(svc, sm, tr))
		r.Get("/workflows/runs", WorkflowRunsPageHandler(svc, sm, tr))
		// Run detail lives under the Workflows surface; the legacy
		// /agents/runs/{shortId} route below still resolves the same handler.
		r.Get("/workflows/runs/{shortId}", AgentRunDetailPageHandler(svc, sm, tr, a.Config.DataDir))
		r.Get("/agents/new", AgentFormPageHandler(svc, sm, tr))
		// /agents/{slug} is the per-agent landing page (lifetime stats +
		// last 10 runs); /agents/{slug}/edit remains the form, reachable
		// from the detail page's Edit button.
		r.Get("/agents/{slug}", AgentDetailPageHandler(svc, sm, tr))
		r.Get("/agents/{slug}/edit", AgentFormPageHandler(svc, sm, tr))
		// A run detail is a workflow run — 301 to the canonical Workflows URL.
		r.Get("/agents/runs/{shortId}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/workflows/runs/"+chi.URLParam(r, "shortId"), http.StatusMovedPermanently)
		})
		r.Get("/agents/runs", func(w http.ResponseWriter, r *http.Request) {
			// Preserve query params so bookmarked filter URLs still work.
			target := "/workflows/runs"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		})
		r.Get("/agents/{slug}/runs", func(w http.ResponseWriter, r *http.Request) {
			slug := chi.URLParam(r, "slug")
			target := "/workflows/runs?workflow=" + slug
			if r.URL.RawQuery != "" {
				target += "&" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		})

		// Prompt library — the v1 wizard. Authors a starter prompt that
		// gets pasted into the SDK agent form.
		r.Get("/agent-prompts", AgentsPageHandler(svc, sm, tr))
		r.Get("/agent-prompts/builder/{type}", PromptBuilderHandler(sm, tr))
		r.Get("/agent-prompts/builder/{type}/copy", PromptCopyHandler())

		r.Get("/logs/sessions/{id}", SessionDetailHandler(svc, sm, tr))

		// Legacy aliases for /agents/sessions/{id} and /activity/sessions/{id}.
		r.Get("/agents/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/logs/sessions/"+chi.URLParam(r, "id"), http.StatusMovedPermanently)
		})
		r.Get("/activity/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/logs/sessions/"+chi.URLParam(r, "id"), http.StatusMovedPermanently)
		})
		// /agent-wizard/{type} legacy → /agent-prompts/builder/{type}.
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

		// Household — promoted to its own top-level page (no longer a
		// tab inside the Settings shell). Lives in the sidebar's System
		// section alongside Logs and Connections.
		r.Route("/household", func(r chi.Router) {
			r.Get("/", UsersListHandler(a, sm, tr))
			r.Get("/new", NewUserHandler(a, sm, tr))
			r.Get("/{id}/edit", EditUserHandler(a, sm, tr))
			r.Get("/{id}/create-login", CreateLoginPageHandler(a, tr))
		})

		// Legacy /settings/household → /household redirects so bookmarks,
		// inbound links from older agent reports, and the keyboard chord
		// `g h` (registered before the move) still resolve.
		r.Get("/settings/household", redirectGET("/household"))
		r.Get("/settings/household/new", redirectGET("/household/new"))
		r.Get("/settings/household/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/household/"+chi.URLParam(r, "id")+"/edit", http.StatusMovedPermanently)
		})
		r.Get("/settings/household/{id}/create-login", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/household/"+chi.URLParam(r, "id")+"/create-login", http.StatusMovedPermanently)
		})

		// Legacy /users redirects.
		r.Get("/users", redirectGET("/household"))
		r.Get("/users/new", redirectGET("/household/new"))
		r.Get("/users/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/household/"+chi.URLParam(r, "id")+"/edit", http.StatusMovedPermanently)
		})
		r.Get("/users/{id}/create-login", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/household/"+chi.URLParam(r, "id")+"/create-login", http.StatusMovedPermanently)
		})

		// Device-code approval page — admin-only because approving mints a
		// new API key. GET shows the form, POST handles approve/deny.
		r.Get("/auth/device", DeviceCodeApprovalHandler(svc, sm))
		r.Post("/auth/device", DeviceCodeApprovalHandler(svc, sm))

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
		// Workflow runtime settings — Claude Agent SDK auth, sidecar, and run
		// ceilings. Admin-only because tokens are sensitive and runs cost.
		r.Get("/settings/workflows", AgentSDKSettingsPageHandler(a, svc, sm, tr))
		// Back-compat: the tab used to live at /settings/agents.
		r.Get("/settings/agents", redirectGET("/settings/workflows"))
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

		// Notifications tab — the outbound notification sink (webhook URL,
		// wire format, public base URL). Admin-only: the sink config governs
		// where household report data egresses. The "Send test" button posts
		// to /-/notifications/test (registered below).
		r.Get("/settings/notifications", NotificationsSettingsHandler(svc, sm, tr))
		r.Post("/settings/notifications", NotificationsSettingsPostHandler(svc, sm))

		r.Get("/settings", SettingsGetHandler(a, sm, tr))
		r.Get("/settings/general", SettingsGetHandler(a, sm, tr))
		r.Get("/settings/sync", redirectGET("/settings/general"))
		r.Get("/settings/security", redirectGET("/settings/system"))
		r.Get("/settings/system", SystemSettingsHandler(a, sm, tr))
		r.Get("/settings/help", HelpSettingsHandler(a, sm, tr))

		// Developer tab — the always-on-top bug/task reporter config
		// (enable toggle + GitHub repo/token/label). Admin-only: the token
		// is sensitive and the flag exposes the reporter household-wide.
		r.Get("/settings/developer", DeveloperSettingsHandler(a, svc, sm, tr))
		r.Post("/settings/developer", DeveloperSettingsPostHandler(a, svc, sm))

		r.Post("/settings/sync", SettingsSyncPostHandler(a, sm))
		r.Post("/settings/retention", SettingsRetentionPostHandler(a, sm))
		r.Post("/settings/avatar-style", SettingsAvatarStylePostHandler(a, sm))
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

		// Developer Mode reporter — file a bug/task issue and serve the
		// durable screenshot / HTML-snapshot artifacts. All roles: the
		// floating reporter renders for anyone once an admin enables
		// developer mode, so anyone who sees it can file.
		r.Post("/dev-reports", CreateDevReportAdminHandler(svc, sm, a.Config.EncryptionKey))
		r.Get("/dev-reports/{shortId}/screenshot", DevReportScreenshotHandler(svc))
		r.Get("/dev-reports/{shortId}/snapshot.html", DevReportSnapshotHandler(svc))

		// Agent run live updates — JSON snapshot the run-detail page
		// polls every 3 s while a run is in_progress. Read-only, all
		// roles (an editor restriction would block dashboards that
		// want to keep an eye on someone else's runs).
		r.Get("/agents/runs/{shortId}/live", AgentRunLiveHandler(svc, sm, tr, a.Config.DataDir))
		// Workflows-surface alias: the run-detail page (served at
		// /workflows/runs/{shortId}) polls this. Same handler; the legacy
		// /agents path above stays for the deferred hand-authored agent pages.
		r.Get("/workflows/runs/{shortId}/live", AgentRunLiveHandler(svc, sm, tr, a.Config.DataDir))

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

			// Agent definitions — editors can CRUD definitions and trigger
			// manual runs. Settings (auth tokens, smoke test, cleanup) are
			// admin-only and registered in the admin-scope group below.
			r.Post("/agents", CreateAgentDefinitionAdminHandler(svc, sm))
			r.Post("/agents/{slug}/update", UpdateAgentDefinitionAdminHandler(svc, sm))
			r.Post("/agents/{slug}/delete", DeleteAgentDefinitionAdminHandler(svc, sm))
			r.Post("/agents/{slug}/enable", EnableAgentAdminHandler(svc))
			// Instantiating a NEW autonomous workflow from a preset is the
			// high-authority act (it authorizes recurring AI spend on shared
			// household data), so it's admin-only — RequireAdmin upgrades this
			// one route above the surrounding editor group. Managing an
			// already-instantiated workflow's run state stays editor.
			r.With(RequireAdmin(sm)).Post("/workflow-presets/{slug}/enable", EnableWorkflowPresetAdminHandler(svc))
			// On-demand (one-off) run: instantiates the manual-only workflow on
			// first use, then dispatches a run. Admin-only for the same reason as
			// enable — it authorizes AI spend over household data.
			r.With(RequireAdmin(sm)).Post("/workflow-presets/{slug}/run", RunWorkflowPresetAdminHandler(a, svc))
			r.Post("/agents/{slug}/disable", DisableAgentAdminHandler(svc))
			r.Post("/agents/{slug}/run", RunAgentNowAdminHandler(a, svc))
			r.Post("/agents/runs/{shortId}/note", UpdateAgentRunNoteAdminHandler(svc, sm))

			// Workflows-surface action aliases (canonical). The Workflows
			// gallery (run toggle), runs tab (re-run), and run-detail page
			// POST to these; the legacy /-/agents/* routes above resolve the
			// same handlers and stay for the deferred hand-authored agent
			// pages. Both sets collapse to one when that surface is removed.
			r.Post("/workflows/{slug}/enable", EnableAgentAdminHandler(svc))
			r.Post("/workflows/{slug}/disable", DisableAgentAdminHandler(svc))
			r.Post("/workflows/{slug}/run", RunAgentNowAdminHandler(a, svc))
			r.Post("/workflows/runs/{shortId}/note", UpdateAgentRunNoteAdminHandler(svc, sm))
			// Lightweight JSON status poll (short_id + status) for the gallery's
			// one-off Run button spinner. Static "runs" segment never shadows
			// the {slug}/* routes above.
			r.Get("/workflows/runs/{shortId}/status", WorkflowRunStatusAdminHandler(svc))

			// Preview the composed internal base prompt for a preset (read-only JSON).
			r.Get("/workflows/{slug}/prompt", WorkflowPromptPreviewAdminHandler(svc))
			// Human-readable preview of a cron expression for the schedule field
			// (read-only JSON). Single-segment path — never shadows {slug}/*.
			r.Get("/workflows/cron-preview", WorkflowCronPreviewAdminHandler(svc))
			// Reconfigure an already-enabled workflow (schedule, additional
			// instructions, options). GET returns the live config to prefill
			// the configure drawer; POST re-composes the prompt + schedule.
			// Both are admin-only — RequireAdmin upgrades them above the
			// surrounding editor group, mirroring the preset-enable guard,
			// because a reconfigure re-authorizes recurring AI spend behavior.
			r.With(RequireAdmin(sm)).Get("/workflows/{slug}/config", WorkflowConfigAdminHandler(svc))
			r.With(RequireAdmin(sm)).Post("/workflows/{slug}/reconfigure", ReconfigureWorkflowAdminHandler(svc))
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
			r.Delete("/connections/{id}", DeleteConnectionHandler(a, sm, svc))
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

			// Agent SDK settings + diagnostics — admin-only because tokens
			// are sensitive (subscription token, Anthropic API key) and
			// smoke-test / cleanup cost money or touch the filesystem.
			r.Post("/agents/settings", UpdateAgentSDKSettingsAdminHandler(a, svc, sm))
			r.Post("/agents/test", SmokeTestAgentAdminHandler(a, svc))
			r.Post("/agents/notify-test", NotifyTestAdminHandler(svc))
			r.Post("/agents/cleanup", AgentCleanupAdminHandler(a, svc))

			// Workflows-surface aliases for the SDK settings + diagnostics
			// (admin-only). The Workflows settings page posts here; the
			// legacy /-/agents/* routes above remain for back-compat.
			r.Post("/workflows/settings", UpdateAgentSDKSettingsAdminHandler(a, svc, sm))
			r.Post("/workflows/test", SmokeTestAgentAdminHandler(a, svc))
			r.Post("/workflows/notify-test", NotifyTestAdminHandler(svc))
			r.Post("/workflows/cleanup", AgentCleanupAdminHandler(a, svc))

			// Notifications tab "Send test" button. Canonical home for the
			// test-delivery endpoint now that the sink lives on its own page;
			// /-/workflows/notify-test stays as a back-compat alias.
			r.Post("/notifications/test", NotifyTestAdminHandler(svc))
		})
	})

	return r
}
