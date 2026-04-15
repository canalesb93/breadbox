package admin

import (
	"net/http"

	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
)

// MCPGuideHandler serves GET /admin/mcp-getting-started.
func MCPGuideHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		data := BaseTemplateData(r, sm, "mcp-getting-started", "Getting Started")

		// Build MCP server URL from request.
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		}
		host := r.Host
		if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
			host = fwdHost
		}
		data["MCPServerURL"] = scheme + "://" + host + "/mcp"

		// Check for existing credentials.
		var hasAPIKeys, hasOAuthClients bool
		if keys, err := svc.ListAPIKeys(ctx); err == nil {
			for _, k := range keys {
				if k.RevokedAt == nil {
					hasAPIKeys = true
					break
				}
			}
		}
		if clients, err := svc.ListOAuthClients(ctx); err == nil {
			for _, c := range clients {
				if c.RevokedAt == nil {
					hasOAuthClients = true
					break
				}
			}
		}
		data["HasAPIKeys"] = hasAPIKeys
		data["HasOAuthClients"] = hasOAuthClients

		// Step 3 contextual data: pending reviews, uncategorized count, rule count.
		var pendingReviews, uncategorizedCount, ruleCount int64
		if n, err := pendingReviewsCount(ctx, svc); err == nil {
			pendingReviews = n
		}
		if cnt, err := svc.CountUncategorizedTransactions(ctx); err == nil {
			uncategorizedCount = cnt
		}
		if result, err := svc.ListTransactionRules(ctx, service.TransactionRuleListParams{Limit: 1}); err == nil {
			ruleCount = int64(result.Total)
		}
		data["PendingReviews"] = pendingReviews
		data["UncategorizedCount"] = uncategorizedCount
		data["RuleCount"] = ruleCount

		tr.Render(w, r, "mcp_guide.html", data)
	}
}
