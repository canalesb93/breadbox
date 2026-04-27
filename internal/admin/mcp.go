package admin

import (
	"net/http"
	"strings"

	breadboxmcp "breadbox/internal/mcp"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
)

// mcpServerURL derives the public MCP endpoint from the incoming request,
// honoring X-Forwarded-Proto / X-Forwarded-Host when set by a reverse proxy.
// Shared across admin pages that surface the URL to users.
func mcpServerURL(r *http.Request) string {
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
	return scheme + "://" + host + "/mcp"
}

// MCPSaveModeHandler handles POST /admin/mcp/mode.
func MCPSaveModeHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mode := r.FormValue("mode")
		if err := svc.SaveMCPMode(r.Context(), mode); err != nil {
			SetFlash(r.Context(), sm, "error", "Invalid mode: must be read_only or read_write")
			http.Redirect(w, r, "/settings/mcp", http.StatusSeeOther)
			return
		}
		SetFlash(r.Context(), sm, "success", "MCP mode updated.")
		http.Redirect(w, r, "/settings/mcp", http.StatusSeeOther)
	}
}

// MCPSaveToolsHandler handles POST /admin/mcp/tools.
func MCPSaveToolsHandler(svc *service.Service, mcpServer *breadboxmcp.MCPServer, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			SetFlash(r.Context(), sm, "error", "Invalid form data")
			http.Redirect(w, r, "/settings/mcp", http.StatusSeeOther)
			return
		}

		// Enabled tools come as form values. Build disabled list from what's NOT checked.
		enabledSet := make(map[string]bool)
		for _, name := range r.Form["enabled_tools"] {
			enabledSet[name] = true
		}

		var disabled []string
		for _, td := range mcpServer.AllToolDefs() {
			if !enabledSet[td.Tool.Name] {
				disabled = append(disabled, td.Tool.Name)
			}
		}

		if err := svc.SaveMCPDisabledTools(r.Context(), disabled); err != nil {
			SetFlash(r.Context(), sm, "error", "Failed to save tool settings")
			http.Redirect(w, r, "/settings/mcp", http.StatusSeeOther)
			return
		}
		SetFlash(r.Context(), sm, "success", "Tool settings updated.")
		http.Redirect(w, r, "/settings/mcp", http.StatusSeeOther)
	}
}

// MCPSaveInstructionsHandler handles POST /admin/mcp/instructions.
func MCPSaveInstructionsHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instructions := strings.TrimSpace(r.FormValue("instructions"))

		if err := svc.SaveMCPInstructions(r.Context(), instructions); err != nil {
			SetFlash(r.Context(), sm, "error", err.Error())
			http.Redirect(w, r, "/settings/mcp", http.StatusSeeOther)
			return
		}
		SetFlash(r.Context(), sm, "success", "Instructions saved.")
		http.Redirect(w, r, "/settings/mcp", http.StatusSeeOther)
	}
}

// MCPSaveReviewGuidelinesHandler handles POST /admin/mcp/review-guidelines.
func MCPSaveReviewGuidelinesHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guidelines := strings.TrimSpace(r.FormValue("review_guidelines"))
		if err := svc.SaveMCPReviewGuidelines(r.Context(), guidelines); err != nil {
			SetFlash(r.Context(), sm, "error", err.Error())
			http.Redirect(w, r, "/settings/mcp#review-guidelines", http.StatusSeeOther)
			return
		}
		SetFlash(r.Context(), sm, "success", "Review guidelines saved.")
		http.Redirect(w, r, "/settings/mcp#review-guidelines", http.StatusSeeOther)
	}
}

// MCPSaveReportFormatHandler handles POST /admin/mcp/report-format.
func MCPSaveReportFormatHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		format := strings.TrimSpace(r.FormValue("report_format"))
		if err := svc.SaveMCPReportFormat(r.Context(), format); err != nil {
			SetFlash(r.Context(), sm, "error", err.Error())
			http.Redirect(w, r, "/settings/mcp#report-format", http.StatusSeeOther)
			return
		}
		SetFlash(r.Context(), sm, "success", "Report format saved.")
		http.Redirect(w, r, "/settings/mcp#report-format", http.StatusSeeOther)
	}
}
