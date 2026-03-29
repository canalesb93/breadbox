package admin

import (
	"net/http"
	"strings"

	breadboxmcp "breadbox/internal/mcp"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
)

// MCPSettingsGetHandler serves GET /admin/mcp.
func MCPSettingsGetHandler(svc *service.Service, mcpServer *breadboxmcp.MCPServer, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := svc.GetMCPConfig(r.Context())
		if err != nil {
			http.Error(w, "Failed to load MCP config", http.StatusInternalServerError)
			return
		}

		// Build tool list for display.
		type toolInfo struct {
			Name           string
			Description    string
			Classification string
			Enabled        bool
			Group          string
		}
		disabledSet := make(map[string]bool)
		for _, t := range cfg.DisabledTools {
			disabledSet[t] = true
		}

		// Tool group assignments (order matters for display).
		toolGroups := map[string]string{
			"list_accounts":                "Accounts & Data",
			"list_users":                   "Accounts & Data",
			"get_sync_status":              "Accounts & Data",
			"trigger_sync":                 "Accounts & Data",
			"query_transactions":           "Transactions",
			"count_transactions":           "Transactions",
			"transaction_summary":          "Transactions",
			"merchant_summary":             "Transactions",
			"list_categories":              "Categories",
			"export_categories":            "Categories",
			"import_categories":            "Categories",
			"categorize_transaction":       "Categorization",
			"reset_transaction_category":   "Categorization",
			"batch_categorize_transactions": "Categorization",
			"bulk_recategorize":            "Categorization",
			"pending_reviews_overview":     "Reviews",
			"list_pending_reviews":         "Reviews",
			"submit_review":               "Reviews",
			"batch_submit_reviews":         "Reviews",
			"list_transaction_rules":       "Rules",
			"create_transaction_rule":      "Rules",
			"update_transaction_rule":      "Rules",
			"delete_transaction_rule":      "Rules",
			"batch_create_rules":           "Rules",
			"apply_rules":                 "Rules",
			"preview_rule":                "Rules",
			"list_account_links":           "Account Links",
			"create_account_link":          "Account Links",
			"delete_account_link":          "Account Links",
			"reconcile_account_link":       "Account Links",
			"list_transaction_matches":     "Account Links",
			"confirm_match":               "Account Links",
			"reject_match":                "Account Links",
			"add_transaction_comment":      "Comments & Reports",
			"list_transaction_comments":    "Comments & Reports",
			"submit_report":               "Comments & Reports",
		}
		// Ordered group names for display.
		groupOrder := []string{
			"Accounts & Data", "Transactions", "Categories", "Categorization",
			"Reviews", "Rules", "Account Links", "Comments & Reports",
		}

		// Build tools grouped in order.
		toolsByGroup := make(map[string][]toolInfo)
		for _, td := range mcpServer.AllToolDefs() {
			enabled := !disabledSet[td.Tool.Name]
			group := toolGroups[td.Tool.Name]
			if group == "" {
				group = "Other"
			}
			toolsByGroup[group] = append(toolsByGroup[group], toolInfo{
				Name:           td.Tool.Name,
				Description:    td.Tool.Description,
				Classification: string(td.Classification),
				Enabled:        enabled,
				Group:          group,
			})
		}
		// Flatten in group order.
		var tools []toolInfo
		for _, g := range groupOrder {
			tools = append(tools, toolsByGroup[g]...)
		}
		if others := toolsByGroup["Other"]; len(others) > 0 {
			tools = append(tools, others...)
		}

		// If no saved instructions, show the defaults.
		instructions := cfg.Instructions
		if instructions == "" {
			instructions = breadboxmcp.DefaultInstructions
		}

		enabledCount := 0
		for _, t := range tools {
			if t.Enabled {
				enabledCount++
			}
		}

		// Build grouped tool data for template.
		type toolGroup struct {
			Name  string
			Tools []toolInfo
		}
		var toolGroupList []toolGroup
		for _, g := range groupOrder {
			if ts := toolsByGroup[g]; len(ts) > 0 {
				toolGroupList = append(toolGroupList, toolGroup{Name: g, Tools: ts})
			}
		}
		if others := toolsByGroup["Other"]; len(others) > 0 {
			toolGroupList = append(toolGroupList, toolGroup{Name: "Other", Tools: others})
		}

		// Resolve review guidelines and report format (custom or default).
		reviewGuidelines := cfg.ReviewGuidelines
		if reviewGuidelines == "" {
			reviewGuidelines = breadboxmcp.DefaultReviewGuidelines
		}
		reportFormat := cfg.ReportFormat
		if reportFormat == "" {
			reportFormat = breadboxmcp.DefaultReportFormat
		}

		data := BaseTemplateData(r, sm, "mcp", "MCP Settings")
		data["MCPConfig"] = cfg
		data["Tools"] = tools
		data["ToolGroups"] = toolGroupList
		data["ToolsEnabledCount"] = enabledCount
		data["ToolsDisabledCount"] = len(tools) - enabledCount
		data["ToolsTotalCount"] = len(tools)
		data["Instructions"] = instructions
		data["DefaultInstructions"] = breadboxmcp.DefaultInstructions
		data["ReviewGuidelines"] = reviewGuidelines
		data["DefaultReviewGuidelines"] = breadboxmcp.DefaultReviewGuidelines
		data["ReportFormat"] = reportFormat
		data["DefaultReportFormat"] = breadboxmcp.DefaultReportFormat

		tr.Render(w, r, "mcp_settings.html", data)
	}
}

// MCPSaveModeHandler handles POST /admin/mcp/mode.
func MCPSaveModeHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mode := r.FormValue("mode")
		if err := svc.SaveMCPMode(r.Context(), mode); err != nil {
			SetFlash(r.Context(), sm, "error", "Invalid mode: must be read_only or read_write")
			http.Redirect(w, r, "/mcp-settings", http.StatusSeeOther)
			return
		}
		SetFlash(r.Context(), sm, "success", "MCP mode updated.")
		http.Redirect(w, r, "/mcp-settings", http.StatusSeeOther)
	}
}

// MCPSaveToolsHandler handles POST /admin/mcp/tools.
func MCPSaveToolsHandler(svc *service.Service, mcpServer *breadboxmcp.MCPServer, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			SetFlash(r.Context(), sm, "error", "Invalid form data")
			http.Redirect(w, r, "/mcp-settings", http.StatusSeeOther)
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
			http.Redirect(w, r, "/mcp-settings", http.StatusSeeOther)
			return
		}
		SetFlash(r.Context(), sm, "success", "Tool settings updated.")
		http.Redirect(w, r, "/mcp-settings", http.StatusSeeOther)
	}
}

// MCPSaveInstructionsHandler handles POST /admin/mcp/instructions.
func MCPSaveInstructionsHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instructions := strings.TrimSpace(r.FormValue("instructions"))

		if err := svc.SaveMCPInstructions(r.Context(), instructions); err != nil {
			SetFlash(r.Context(), sm, "error", err.Error())
			http.Redirect(w, r, "/mcp-settings", http.StatusSeeOther)
			return
		}
		SetFlash(r.Context(), sm, "success", "Instructions saved.")
		http.Redirect(w, r, "/mcp-settings", http.StatusSeeOther)
	}
}

// MCPSaveReviewGuidelinesHandler handles POST /admin/mcp/review-guidelines.
func MCPSaveReviewGuidelinesHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guidelines := strings.TrimSpace(r.FormValue("review_guidelines"))
		if err := svc.SaveMCPReviewGuidelines(r.Context(), guidelines); err != nil {
			SetFlash(r.Context(), sm, "error", err.Error())
			http.Redirect(w, r, "/mcp-settings#review-guidelines", http.StatusSeeOther)
			return
		}
		SetFlash(r.Context(), sm, "success", "Review guidelines saved.")
		http.Redirect(w, r, "/mcp-settings#review-guidelines", http.StatusSeeOther)
	}
}

// MCPSaveReportFormatHandler handles POST /admin/mcp/report-format.
func MCPSaveReportFormatHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		format := strings.TrimSpace(r.FormValue("report_format"))
		if err := svc.SaveMCPReportFormat(r.Context(), format); err != nil {
			SetFlash(r.Context(), sm, "error", err.Error())
			http.Redirect(w, r, "/mcp-settings#report-format", http.StatusSeeOther)
			return
		}
		SetFlash(r.Context(), sm, "success", "Report format saved.")
		http.Redirect(w, r, "/mcp-settings#report-format", http.StatusSeeOther)
	}
}
