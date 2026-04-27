package admin

import (
	"net/http"

	breadboxmcp "breadbox/internal/mcp"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// AgentsSettingsHandler serves GET /admin/settings/mcp — the MCP tab
// inside the unified Settings shell. Hosts the four MCP-config cards
// (Server Instructions, Review Guidelines, Report Format, Tools Enabled)
// plus the read-only Connection cheatsheet.
//
// POST targets remain at /-/mcp-settings/* — only the GET page moved.
func AgentsSettingsHandler(svc *service.Service, mcpServer *breadboxmcp.MCPServer, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		cfg, err := svc.GetMCPConfig(ctx)
		if err != nil {
			http.Error(w, "Failed to load MCP config", http.StatusInternalServerError)
			return
		}

		disabledSet := make(map[string]bool, len(cfg.DisabledTools))
		for _, t := range cfg.DisabledTools {
			disabledSet[t] = true
		}

		// Group ordering matches the previous MCP-settings layout
		// so user muscle memory carries over.
		toolGroups := map[string]string{
			"list_accounts":                 "Accounts & Data",
			"list_users":                    "Accounts & Data",
			"get_sync_status":               "Accounts & Data",
			"trigger_sync":                  "Accounts & Data",
			"query_transactions":            "Transactions",
			"count_transactions":            "Transactions",
			"transaction_summary":           "Transactions",
			"merchant_summary":              "Transactions",
			"list_categories":               "Categories",
			"export_categories":             "Categories",
			"import_categories":             "Categories",
			"categorize_transaction":        "Categorization",
			"reset_transaction_category":    "Categorization",
			"batch_categorize_transactions": "Categorization",
			"bulk_recategorize":             "Categorization",
			"list_tags":                     "Tags",
			"create_tag":                    "Tags",
			"update_tag":                    "Tags",
			"delete_tag":                    "Tags",
			"add_transaction_tag":           "Tags",
			"remove_transaction_tag":        "Tags",
			"update_transactions":           "Tags",
			"list_annotations":              "Tags",
			"list_transaction_rules":        "Rules",
			"create_transaction_rule":       "Rules",
			"update_transaction_rule":       "Rules",
			"delete_transaction_rule":       "Rules",
			"batch_create_rules":            "Rules",
			"apply_rules":                   "Rules",
			"preview_rule":                  "Rules",
			"list_account_links":            "Account Links",
			"create_account_link":           "Account Links",
			"delete_account_link":           "Account Links",
			"reconcile_account_link":        "Account Links",
			"list_transaction_matches":      "Account Links",
			"confirm_match":                 "Account Links",
			"reject_match":                  "Account Links",
			"add_transaction_comment":       "Comments & Reports",
			"list_transaction_comments":     "Comments & Reports",
			"submit_report":                 "Comments & Reports",
			"create_session":                "Session",
		}
		groupOrder := []string{
			"Session", "Accounts & Data", "Transactions", "Categories", "Categorization",
			"Tags", "Rules", "Account Links", "Comments & Reports",
		}

		toolsByGroup := make(map[string][]pages.AgentsSettingsTool)
		var totalCount, enabledCount int
		for _, td := range mcpServer.AllToolDefs() {
			isEnabled := !disabledSet[td.Tool.Name]
			group := toolGroups[td.Tool.Name]
			if group == "" {
				group = "Other"
			}
			toolsByGroup[group] = append(toolsByGroup[group], pages.AgentsSettingsTool{
				Name:           td.Tool.Name,
				Description:    td.Tool.Description,
				Classification: string(td.Classification),
				Enabled:        isEnabled,
			})
			totalCount++
			if isEnabled {
				enabledCount++
			}
		}

		var groups []pages.AgentsSettingsToolGroup
		for _, g := range groupOrder {
			if ts := toolsByGroup[g]; len(ts) > 0 {
				groups = append(groups, pages.AgentsSettingsToolGroup{Name: g, Tools: ts})
			}
		}
		if others := toolsByGroup["Other"]; len(others) > 0 {
			groups = append(groups, pages.AgentsSettingsToolGroup{Name: "Other", Tools: others})
		}

		instructions := cfg.Instructions
		if instructions == "" {
			instructions = breadboxmcp.DefaultInstructions
		}
		reviewGuidelines := cfg.ReviewGuidelines
		if reviewGuidelines == "" {
			reviewGuidelines = breadboxmcp.DefaultReviewGuidelines
		}
		reportFormat := cfg.ReportFormat
		if reportFormat == "" {
			reportFormat = breadboxmcp.DefaultReportFormat
		}

		props := pages.AgentsSettingsProps{
			CSRFToken:               GetCSRFToken(r),
			Instructions:            instructions,
			DefaultInstructions:     breadboxmcp.DefaultInstructions,
			ReviewGuidelines:        reviewGuidelines,
			DefaultReviewGuidelines: breadboxmcp.DefaultReviewGuidelines,
			ReportFormat:            reportFormat,
			DefaultReportFormat:     breadboxmcp.DefaultReportFormat,
			ToolGroups:              groups,
			ToolsEnabledCount:       enabledCount,
			ToolsTotalCount:         totalCount,
			ToolsDisabledCount:      totalCount - enabledCount,
		}

		data := BaseTemplateData(r, sm, "mcp", "MCP Settings")
		renderSettingsTab(tr, w, r, sm, data, pages.SettingsTabAgents, pages.AgentsSettings(props))
	}
}
