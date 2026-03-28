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

		tr.Render(w, r, "mcp_guide.html", data)
	}
}
