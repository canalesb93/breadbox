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
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))

		// Read endpoints — all API keys.
		r.Get("/accounts", ListAccountsHandler(svc))
		r.Get("/accounts/{id}", GetAccountHandler(svc))
		r.Get("/transactions", ListTransactionsHandler(svc))
		r.Get("/transactions/count", CountTransactionsHandler(svc))
		r.Get("/transactions/summary", TransactionSummaryHandler(svc))
		r.Get("/transactions/{id}", GetTransactionHandler(svc))
		r.Get("/categories", ListCategoriesHandler(svc))
		r.Get("/categories/unmapped", ListUnmappedCategoriesHandler(svc))
		r.Get("/categories/export", ExportCategoriesTSVHandler(svc))
		r.Get("/categories/{id}", GetCategoryHandler(svc))
		r.Get("/category-mappings", ListMappingsHandler(svc))
		r.Get("/category-mappings/export", ExportMappingsTSVHandler(svc))
		r.Get("/users", ListUsersHandler(svc))
		r.Get("/connections", ListConnectionsHandler(svc))
		r.Get("/connections/{id}/status", GetConnectionStatusHandler(svc))
		r.Get("/transactions/{transaction_id}/comments", ListCommentsHandler(svc))
		r.Get("/reviews", ListReviewsHandler(svc))
		r.Get("/reviews/counts", ReviewCountsHandler(svc))
		r.Get("/reviews/summary", ReviewSummaryHandler(svc))
		r.Get("/reviews/{id}", GetReviewHandler(svc))
		r.Get("/rules", ListRulesHandler(svc))
		r.Get("/rules/{id}", GetRuleHandler(svc))
		r.Get("/account-links", ListAccountLinksHandler(svc))
		r.Get("/account-links/{id}", GetAccountLinkHandler(svc))
		r.Get("/account-links/{id}/matches", ListTransactionMatchesHandler(svc))
		r.Get("/reports", ListReportsHandler(svc))
		r.Get("/reports/unread-count", UnreadReportCountHandler(svc))

		// Write endpoints — full_access API keys only.
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Patch("/transactions/{id}/category", SetTransactionCategoryHandler(svc))
			r.Delete("/transactions/{id}/category", ResetTransactionCategoryHandler(svc))
			r.Post("/categories", CreateCategoryHandler(svc))
			r.Post("/categories/import", ImportCategoriesTSVHandler(svc))
			r.Put("/categories/{id}", UpdateCategoryHandler(svc))
			r.Delete("/categories/{id}", DeleteCategoryHandler(svc))
			r.Post("/categories/{id}/merge", MergeCategoriesHandler(svc))
			r.Put("/category-mappings", BulkUpsertMappingsHandler(svc))
			r.Post("/category-mappings/import", ImportMappingsTSVHandler(svc))
			r.Delete("/category-mappings/{id}", DeleteMappingHandler(svc))
			r.Post("/sync", TriggerSyncHandler(svc))
			r.Post("/transactions/{transaction_id}/comments", CreateCommentHandler(svc))
			r.Put("/transactions/{transaction_id}/comments/{id}", UpdateCommentHandler(svc))
			r.Delete("/transactions/{transaction_id}/comments/{id}", DeleteCommentHandler(svc))
			r.Post("/reviews/{id}/submit", SubmitReviewHandler(svc))
			r.Post("/reviews/bulk", BulkSubmitReviewsHandler(svc))
			r.Post("/reviews/enqueue", EnqueueReviewHandler(svc))
			r.Post("/reviews/auto-approve", AutoApproveCategorizedHandler(svc))
			r.Delete("/reviews/{id}", DismissReviewHandler(svc))
			r.Post("/rules", CreateRuleHandler(svc))
			r.Put("/rules/{id}", UpdateRuleHandler(svc))
			r.Delete("/rules/{id}", DeleteRuleHandler(svc))
			r.Post("/rules/{id}/apply", ApplyRuleHandler(svc))
			r.Post("/rules/apply-all", ApplyAllRulesHandler(svc))
			r.Post("/rules/preview", PreviewRuleHandler(svc))
			r.Post("/transactions/batch-categorize", BatchCategorizeHandler(svc))
			r.Post("/transactions/bulk-recategorize", BulkRecategorizeHandler(svc))
			r.Post("/account-links", CreateAccountLinkHandler(svc))
			r.Put("/account-links/{id}", UpdateAccountLinkHandler(svc))
			r.Delete("/account-links/{id}", DeleteAccountLinkHandler(svc))
			r.Post("/account-links/{id}/reconcile", ReconcileAccountLinkHandler(svc))
			r.Post("/transaction-matches/{id}/confirm", ConfirmMatchHandler(svc))
			r.Post("/transaction-matches/{id}/reject", RejectMatchHandler(svc))
			r.Post("/transaction-matches/manual", ManualMatchHandler(svc))
			r.Post("/reports", CreateReportHandler(svc))
			r.Patch("/reports/{id}/read", MarkReportReadHandler(svc))
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

	// Webhook handler — no auth (verified via JWT in provider).
	r.Post("/webhooks/{provider}", webhook.NewHandler(a.Providers, a.SyncEngine, a.Queries, a.Logger))

	// OAuth 2.1 discovery endpoints — no auth required.
	r.Get("/.well-known/oauth-authorization-server", admin.OAuthMetadataHandler())
	r.Get("/.well-known/oauth-protected-resource", admin.OAuthProtectedResourceHandler())

	// OAuth 2.1 token + registration endpoints — no session required.
	r.Post("/oauth/token", admin.OAuthTokenHandler(svc))
	r.Post("/oauth/register", admin.OAuthRegisterHandler(svc))

	// Admin dashboard: session manager + template renderer + admin router.
	isSecure := a.Config.Environment == "production" || a.Config.Environment == "docker"
	sm := admin.NewSessionManager(a.DB, isSecure)
	tr, err := admin.NewTemplateRenderer(sm)
	if err != nil {
		a.Logger.Error("failed to initialize template renderer", "error", err)
	} else {
		tr.SetVersion(a.Config.Version)
		adminRouter := admin.NewAdminRouter(a, sm, tr, svc, mcpServer)
		r.Mount("/", adminRouter)
	}

	return r
}
