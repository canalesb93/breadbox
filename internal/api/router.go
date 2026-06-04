//go:build !lite

package api

import (
	"io/fs"
	"net/http"

	"breadbox/internal/admin"
	"breadbox/internal/app"
	breadboxmcp "breadbox/internal/mcp"
	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
	"breadbox/internal/webhook"
	"breadbox/static"

	"github.com/alexedwards/scs/v2"
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

	// Static files (CSS, favicon) — embedded, no auth needed.
	staticFS, _ := fs.Sub(static.FS, ".")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	r.Get("/health", HealthLiveHandler(version))
	r.Get("/health/live", HealthLiveHandler(version))
	r.Get("/health/ready", HealthReadyHandler(a, version))

	// Version check — no auth required.
	r.Get("/api/v1/version", VersionHandler(a.VersionChecker, version))

	// REST API v1 — API key authenticated.
	svc := service.New(a.Queries, a.DB, a.SyncEngine, a.Logger)
	svc.EncryptionKey = a.Config.EncryptionKey
	apiLimiter := mw.NewRateLimiter(mw.RateLimitConfig{
		RequestsPerMinute: a.Config.APIRateLimitRPM,
		Burst:             a.Config.APIRateLimitBurst,
	})

	// Device-code auth — unauthenticated; the device_code is itself the
	// credential the CLI carries while it waits for a browser approval.
	// Mounted outside the /api/v1 Route block so APIKeyAuth doesn't
	// intercept the unauthenticated polling loop. Surfaced in
	// openapi.yaml; the drift test's healthVersionRoutes allowlist keeps
	// it in sync.
	r.Post("/api/v1/auth/device-code", CreateDeviceCodeHandler(svc))
	r.Post("/api/v1/auth/device-code/poll", PollDeviceCodeHandler(svc))

	// Session manager — created once, shared by /api/v1 (the cookie-auth
	// fallback) and the dashboard mount below. nil under --no-dashboard
	// (and -tags=headless): there are no cookie clients, so /api/v1 then
	// behaves exactly as before — API-key / Bearer only.
	var sm *scs.SessionManager
	if !a.Config.NoDashboard {
		sm = admin.NewSessionManager(a.DB)
	}

	r.Route("/api/v1", func(r chi.Router) {
		// Auth runs first so the rate limiter can identify by API key ID
		// (a session is translated into a synthetic key keyed by the user).
		// /health/* and /api/v1/version are mounted outside this Route block
		// and are intentionally not rate-limited (cheap, used by load
		// balancers / monitoring).
		if sm != nil {
			// Stamp Secure on the session cookie per request — must wrap the
			// writer scs sets the cookie on, so it goes before LoadAndSave.
			r.Use(mw.SecureSessionCookie(a.Config.SecureCookies, sm.Cookie.Name))
			r.Use(sm.LoadAndSave)
		}
		r.Use(sessionOrAPIKeyAuth(svc, sm))
		r.Use(apiLimiter.Middleware())

		// Read endpoints — all API keys.
		r.Get("/accounts", ListAccountsHandler(svc))
		r.Get("/accounts/{id}", GetAccountHandler(svc))
		r.Get("/accounts/{id}/detail", GetAccountDetailHandler(svc))
		r.Get("/transactions", ListTransactionsHandler(svc))
		r.Get("/transactions/count", CountTransactionsHandler(svc))
		r.Get("/transactions/summary", TransactionSummaryHandler(svc))
		r.Get("/transactions/merchants", MerchantSummaryHandler(svc))
		r.Get("/transactions/{id}", GetTransactionHandler(svc))
		r.Get("/categories", ListCategoriesHandler(svc))
		r.Get("/categories/export", ExportCategoriesTSVHandler(svc))
		r.Get("/categories/{id}", GetCategoryHandler(svc))
		r.Get("/users", ListUsersHandler(svc))
		r.Get("/users/{id}", GetUserHandler(svc))
		r.Get("/connections", ListConnectionsHandler(svc))
		r.Get("/connections/{id}", GetConnectionHandler(svc))
		r.Get("/connections/{id}/status", GetConnectionStatusHandler(svc))
		r.Get("/connections/link/{id}", GetHostedLinkSessionHandler(svc))
		r.Get("/sync/logs", ListSyncLogsHandler(svc))
		r.Get("/sync/logs/{id}", GetSyncLogHandler(svc))
		r.Get("/sync/health", SyncHealthHandler(svc))
		r.Get("/sync/health/providers", SyncProviderHealthHandler(svc))
		r.Get("/sync/stats", SyncStatsHandler(svc))
		r.Get("/transactions/{transaction_id}/comments", ListCommentsHandler(svc))
		r.Get("/transactions/{id}/annotations", ListAnnotationsHandler(svc))
		r.Get("/rules", ListRulesHandler(svc))
		r.Get("/rules/{id}", GetRuleHandler(svc))
		r.Get("/rules/{id}/sync-history", GetRuleSyncHistoryHandler(svc))
		r.Get("/account-links", ListAccountLinksHandler(svc))
		r.Get("/account-links/{id}", GetAccountLinkHandler(svc))
		r.Get("/account-links/{id}/matches", ListTransactionMatchesHandler(svc))
		r.Get("/reports", ListReportsHandler(svc))
		r.Get("/reports/unread-count", UnreadReportCountHandler(svc))
		r.Get("/reports/{id}", GetReportHandler(svc))
		r.Get("/tags", ListTagsHandler(svc))
		r.Get("/tags/{slug}", GetTagHandler(svc))
		r.Get("/series", ListSeriesHandler(svc))
		r.Get("/series/explain", ExplainSeriesCandidatesHandler(svc)) // static before /series/{id}
		r.Get("/series/{id}", GetSeriesHandler(svc))
		r.Get("/settings/providers", GetProviderConfigHandler(a))
		// Agents — read endpoints. Specific paths before /workflows/{slug} param.
		r.Get("/workflows", ListAgentDefinitionsHandler(svc))
		r.Get("/workflows/settings", GetAgentSettingsHandler(svc, a))
		r.Get("/workflows/status", AgentSubsystemStatusHandler(svc))
		r.Get("/workflows/prompt-blocks", ListPromptBlocksHandler(svc))
		r.Get("/workflows/runs", ListAllAgentRunsHandler(svc))
		r.Get("/workflows/runs/recent-errors", ListRecentErroredAgentRunsHandler(svc))
		r.Get("/workflows/runs/{shortId}", GetAgentRunHandler(svc))
		r.Get("/workflows/runs/{shortId}/transcript", GetAgentRunTranscriptHandler(svc, a))
		r.Get("/workflows/{slug}", GetAgentDefinitionHandler(svc))
		r.Get("/workflows/{slug}/runs", ListAgentRunsHandler(svc))
		r.Get("/workflow-presets", ListWorkflowPresetsHandler(svc))
		r.Get("/providers", ListProvidersHandler(a))
		r.Get("/providers/{name}", GetProviderHandler(a))
		r.Get("/headless/bootstrap", HeadlessBootstrapHandler(svc, a, version))
		r.Get("/keys/me", WhoamiHandler())

		// Write endpoints — full_access API keys only.
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Patch("/accounts/{id}", UpdateAccountHandler(svc))
			r.Patch("/transactions/{id}/category", SetTransactionCategoryHandler(svc))
			r.Delete("/transactions/{id}/category", ResetTransactionCategoryHandler(svc))
			r.Post("/categories", CreateCategoryHandler(svc))
			r.Post("/categories/import", ImportCategoriesTSVHandler(svc))
			r.Put("/categories/{id}", UpdateCategoryHandler(svc))
			r.Delete("/categories/{id}", DeleteCategoryHandler(svc))
			r.Post("/categories/{id}/merge", MergeCategoriesHandler(svc))
			r.Post("/sync", TriggerSyncHandler(svc))
			r.Post("/connections/{id}/sync", SyncConnectionHandler(svc))
			r.Post("/connections/{id}/paused", PauseConnectionHandler(svc))
			r.Post("/connections/{id}/sync-interval", UpdateConnectionSyncIntervalHandler(svc))
			r.Delete("/connections/{id}", DeleteConnectionHandler(svc))
			r.Post("/connections/{id}/reauth", ConnectionReauthHandler(a))
			r.Post("/connections/{id}/reauth-complete", ConnectionReauthCompleteHandler(a))
			r.Post("/connections/plaid/link-token", PlaidLinkTokenHandler(a))
			r.Post("/connections/plaid/exchange", PlaidExchangeHandler(a))
			r.Post("/connections/teller", TellerSetupHandler(a))
			r.Post("/transactions/{transaction_id}/comments", CreateCommentHandler(svc))
			r.Put("/transactions/{transaction_id}/comments/{id}", UpdateCommentHandler(svc))
			r.Delete("/transactions/{transaction_id}/comments/{id}", DeleteCommentHandler(svc))
			r.Post("/rules", CreateRuleHandler(svc))
			r.Post("/rules/batch", BatchCreateRulesHandler(svc))
			r.Put("/rules/{id}", UpdateRuleHandler(svc))
			r.Delete("/rules/{id}", DeleteRuleHandler(svc))
			r.Post("/rules/{id}/apply", ApplyRuleHandler(svc))
			r.Post("/rules/apply-all", ApplyAllRulesHandler(svc))
			r.Post("/rules/preview", PreviewRuleHandler(svc))
			r.Post("/transactions/batch-categorize", BatchCategorizeHandler(svc))
			r.Post("/transactions/bulk-recategorize", BulkRecategorizeHandler(svc))
			r.Post("/transactions/update", UpdateTransactionsHandler(svc))
			r.Delete("/transactions/{id}", DeleteTransactionHandler(svc))
			r.Post("/transactions/{id}/restore", RestoreTransactionHandler(svc))
			r.Post("/account-links", CreateAccountLinkHandler(svc))
			r.Put("/account-links/{id}", UpdateAccountLinkHandler(svc))
			r.Delete("/account-links/{id}", DeleteAccountLinkHandler(svc))
			r.Post("/account-links/{id}/reconcile", ReconcileAccountLinkHandler(svc))
			r.Post("/transaction-matches/{id}/confirm", ConfirmMatchHandler(svc))
			r.Post("/transaction-matches/{id}/reject", RejectMatchHandler(svc))
			r.Post("/transaction-matches/manual", ManualMatchHandler(svc))
			r.Post("/reports", CreateReportHandler(svc))
			r.Patch("/reports/{id}/read", MarkReportReadHandler(svc))
			r.Patch("/reports/{id}/unread", MarkReportUnreadHandler(svc))
			r.Post("/reports/read-all", MarkAllReportsReadHandler(svc))
			r.Delete("/reports/{id}", DeleteReportHandler(svc))
			r.Get("/api-keys", ListAPIKeysHandler(svc))
			r.Post("/api-keys", CreateAPIKeyHandler(svc))
			r.Delete("/api-keys/{id}", RevokeAPIKeyHandler(svc))
			r.Post("/transactions/{id}/tags", AddTransactionTagHandler(svc))
			r.Delete("/transactions/{id}/tags/{slug}", RemoveTransactionTagHandler(svc))
			r.Patch("/transactions/{id}/metadata/{key}", SetTransactionMetadataKeyHandler(svc))
			r.Delete("/transactions/{id}/metadata/{key}", RemoveTransactionMetadataKeyHandler(svc))
			r.Put("/transactions/{id}/metadata", ReplaceTransactionMetadataHandler(svc))
			r.Delete("/transactions/{id}/metadata", ClearTransactionMetadataHandler(svc))
			r.Post("/transactions/{id}/flag", FlagTransactionHandler(svc))
			r.Delete("/transactions/{id}/flag", UnflagTransactionHandler(svc))
			r.Post("/tags", CreateTagHandler(svc))
			r.Patch("/tags/{slug}", UpdateTagHandler(svc))
			r.Post("/series", AssignSeriesHandler(svc))
			r.Post("/series/{id}/transactions", LinkSeriesTransactionsHandler(svc))
			r.Delete("/series/{id}/transactions/{txid}", UnlinkSeriesTransactionHandler(svc))
			r.Post("/series/{id}/rekey", RekeySeriesHandler(svc))
			r.Post("/series/{id}/split", SplitSeriesHandler(svc))
			r.Post("/series/{id}/type", SetSeriesTypeHandler(svc))
			r.Post("/series/{id}/tags", AddSeriesTagHandler(svc))
			r.Delete("/series/{id}/tags/{slug}", RemoveSeriesTagHandler(svc))
			r.Patch("/series/{id}", PatchSeriesHandler(svc))
			r.Delete("/tags/{slug}", DeleteTagHandler(svc))
			r.Put("/settings/providers/plaid", UpdatePlaidConfigHandler(a))
			r.Put("/settings/providers/teller", UpdateTellerConfigHandler(a))
			// Agents — write endpoints (full_access scope).
			r.Post("/workflows", CreateAgentDefinitionHandler(svc))
			r.Post("/workflow-presets/{slug}/enable", EnableWorkflowPresetHandler(svc))
			r.Put("/workflows/settings", UpdateAgentSettingsHandler(svc, a))
			r.Patch("/workflows/{slug}", UpdateAgentDefinitionHandler(svc))
			r.Delete("/workflows/{slug}", DeleteAgentDefinitionHandler(svc))
			r.Post("/workflows/{slug}/enable", EnableAgentHandler(svc))
			r.Post("/workflows/{slug}/disable", DisableAgentHandler(svc))
			r.Post("/workflows/{slug}/run", RunAgentNowHandler(svc, a.AgentOrchestrator))
			r.Post("/workflows/test", SmokeTestAgentHandler(a.AgentOrchestrator))
			r.Post("/workflows/cleanup", RunAgentCleanupHandler(a.AgentScheduler))
			r.Patch("/workflows/runs/{shortId}", UpdateAgentRunNoteHandler(svc))
			r.Post("/providers/{name}/test", TestProviderHandler(a))
			r.Delete("/providers/{name}", DisableProviderHandler(a))
			r.Get("/config", ListConfigHandler(a))
			r.Get("/config/{key}", GetConfigHandler(a))
			r.Put("/config/{key}", SetConfigHandler(a))
			r.Delete("/config/{key}", DeleteConfigHandler(a))
			r.Get("/webhook-events", ListWebhookEventsHandler(svc))
			r.Post("/webhook-events/{id}/replay", ReplayWebhookEventHandler(svc, a.SyncEngine))
			r.Post("/users", CreateUserHandler(svc))
			r.Patch("/users/{id}", UpdateUserHandler(svc))
			r.Delete("/users/{id}", DeleteUserHandler(svc))
			r.Post("/users/{id}/wipe-data", WipeUserDataHandler(svc))
			r.Get("/users/{user_id}/login", ListUserLoginsHandler(svc))
			r.Post("/users/{user_id}/login", CreateUserLoginHandler(svc))
			r.Patch("/users/{user_id}/login/{login_id}", UpdateUserLoginHandler(svc))
			r.Delete("/users/{user_id}/login/{login_id}", DeleteUserLoginHandler(svc))
			r.Post("/users/{user_id}/login/{login_id}/regenerate-token", RegenerateLoginTokenHandler(svc))
			r.Get("/login-accounts", ListLoginAccountsHandler(svc))
			r.Delete("/login-accounts/{id}", DeleteLoginAccountHandler(svc))
			r.Post("/login-accounts/{id}/reset-password", ResetLoginAccountPasswordHandler(svc))
			r.Post("/connections/csv/preview", CSVPreviewHandler(svc))
			r.Post("/connections/csv/import", CSVImportHandler(svc))
			r.Post("/connections/link", CreateHostedLinkHandler(svc))
			r.Post("/connections/{id}/relink", CreateHostedLinkRelinkHandler(svc))
			// Generic provider create + link-session — supersede the
			// per-provider routes above. The old routes remain wired as
			// deprecated shims (callers will see identical behavior).
			r.Post("/providers/{name}/link-session", LinkSessionHandler(a))
			r.Post("/connections", CreateConnectionHandler(a))
		})
	})

	// MCP server — API key authenticated, per-request filtering.
	mcpServer := breadboxmcp.NewMCPServer(svc, version)
	mcpHandler := breadboxmcp.NewHTTPHandler(mcpServer, svc)
	r.Group(func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))
		r.Handle("/mcp", mcpHandler)
		r.Handle("/mcp/*", mcpHandler)
	})

	// Hosted-link page surface — see internal/api/hosted_link_page.go.
	//
	// GET /link/{token}      → standalone HTML page (no auth)
	// /_link/{token}/*       → page-internal JSON endpoints (bearer-gated)
	//
	// These are NOT under /api/v1 — they're a separate surface, intentionally
	// not modeled in openapi.yaml. The drift test scopes itself to /api/v1/*
	// and ignores everything mounted at the root (these, /webhooks, /mcp).
	r.Get("/link/{token}", HostedLinkPageHandler())
	r.Route("/_link/{token}", func(r chi.Router) {
		r.Use(mw.HostedLinkBearer(svc))
		r.Get("/session", GetHostedLinkPageSessionHandler(svc))
		r.Post("/providers/{name}/start", HostedLinkPageStartHandler(a))
		r.Get("/providers/teller/config", HostedLinkPageTellerConfigHandler(a))
		r.Post("/connections", HostedLinkPageConnectionHandler(a))
		r.Post("/reauth-complete", HostedLinkPageReauthCompleteHandler(svc))
		r.Post("/complete", HostedLinkPageCompleteHandler(svc))
		r.Post("/fail", HostedLinkPageFailHandler(svc))
	})

	// Webhook handler — no auth (verified via JWT in provider).
	r.Post("/webhooks/{provider}", webhook.NewHandler(a.Providers, a.SyncEngine, a.Queries, a.Logger))

	// OAuth 2.1 discovery endpoints — no auth required.
	r.Get("/.well-known/oauth-authorization-server", admin.OAuthMetadataHandler())
	r.Get("/.well-known/oauth-protected-resource", admin.OAuthProtectedResourceHandler())

	// OAuth 2.1 token + registration endpoints — no session required.
	r.Post("/oauth/token", admin.OAuthTokenHandler(svc))
	r.Post("/oauth/register", admin.OAuthRegisterHandler(svc))

	// Admin dashboard — gated by the runtime --no-dashboard flag. When
	// disabled, REST + MCP + OAuth + webhooks stay reachable; the dashboard
	// surface is silently absent (no admin router mounted on "/" — bare
	// GET / returns 404). The build-tag side that strips the assets
	// entirely is `-tags=headless` (see .claude/rules/build-tags.md).
	if !a.Config.NoDashboard {
		// sm was created above (shared with the /api/v1 cookie-auth path).
		tr, err := admin.NewTemplateRenderer(sm)
		if err != nil {
			a.Logger.Error("failed to initialize template renderer", "error", err)
		} else {
			tr.SetVersion(a.Config.Version)
			tr.SetVersionChecker(a.VersionChecker)
			tr.SetAppConfigReader(a.Queries)
			adminRouter := admin.NewAdminRouter(a, sm, tr, svc, mcpServer)
			r.Mount("/", adminRouter)
		}
	}

	return r
}
