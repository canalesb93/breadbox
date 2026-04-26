package admin

import (
	"net/http"
	"time"

	breadboxmcp "breadbox/internal/mcp"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// relativeTimeFromRFC3339 parses an RFC3339 timestamp and returns a
// human-readable relative-time string. Empty input returns empty string.
func relativeTimeFromRFC3339(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ""
	}
	return relativeTime(t)
}

// AgentWizardStats holds live stats shown on the agent wizard cards.
type AgentWizardStats struct {
	PendingReviews int64
	TotalRules     int64
	TotalAccounts  int64
}

// AgentsPageHandler serves GET /admin/agents — combined Getting Started
// + Agent Wizard + MCP Settings + Activity. Renders pages.Agents via
// the templ shell. The four panes were previously
// pages/{agents,mcp_guide,agent_wizard,mcp_settings}.html wired together
// through admin/templates.go's compositePages map, which this PR removes.
func AgentsPageHandler(svc *service.Service, mcpServer *breadboxmcp.MCPServer, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tab := pages.AgentsTab(r.URL.Query().Get("tab"))
		switch tab {
		case pages.AgentsTabWizard, pages.AgentsTabSettings, pages.AgentsTabActivity, pages.AgentsTabGuide:
		default:
			tab = pages.AgentsTabGuide
		}

		data := BaseTemplateData(r, sm, "agents", "Agents")

		// === Getting Started data ===
		guide := pages.MCPGuideProps{
			MCPServerURL: mcpServerURL(r),
		}
		if keys, err := svc.ListAPIKeys(ctx); err == nil {
			for _, k := range keys {
				if k.RevokedAt == nil {
					guide.HasAPIKeys = true
					break
				}
			}
		}
		if clients, err := svc.ListOAuthClients(ctx); err == nil {
			for _, c := range clients {
				if c.RevokedAt == nil {
					guide.HasOAuthClients = true
					break
				}
			}
		}

		var pendingReviews, ruleCount int64
		if n, err := pendingReviewsCount(ctx, svc); err == nil {
			pendingReviews = n
		}
		enabled := true
		if result, err := svc.ListTransactionRules(ctx, service.TransactionRuleListParams{Enabled: &enabled, Limit: 1}); err == nil {
			ruleCount = int64(result.Total)
		}

		// === Agent Wizard data ===
		var totalAccounts int64
		if accounts, err := svc.ListAccounts(ctx, nil); err == nil {
			totalAccounts = int64(len(accounts))
		}
		wizard := pages.AgentWizardProps{
			Stats: pages.AgentWizardStatsProps{
				PendingReviews: pendingReviews,
				TotalRules:     ruleCount,
				TotalAccounts:  totalAccounts,
			},
		}

		// === MCP Settings data ===
		cfg, err := svc.GetMCPConfig(ctx)
		if err != nil {
			http.Error(w, "Failed to load MCP config", http.StatusInternalServerError)
			return
		}

		disabledSet := make(map[string]bool)
		for _, t := range cfg.DisabledTools {
			disabledSet[t] = true
		}

		toolGroupsMap := map[string]string{
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

		toolsByGroup := make(map[string][]pages.MCPSettingsToolInfo)
		var totalTools, enabledCount int
		for _, td := range mcpServer.AllToolDefs() {
			isEnabled := !disabledSet[td.Tool.Name]
			if isEnabled {
				enabledCount++
			}
			totalTools++
			group := toolGroupsMap[td.Tool.Name]
			if group == "" {
				group = "Other"
			}
			toolsByGroup[group] = append(toolsByGroup[group], pages.MCPSettingsToolInfo{
				Name:           td.Tool.Name,
				Description:    td.Tool.Description,
				Classification: string(td.Classification),
				Enabled:        isEnabled,
			})
		}
		var toolGroupList []pages.MCPSettingsToolGroup
		for _, g := range groupOrder {
			if ts := toolsByGroup[g]; len(ts) > 0 {
				toolGroupList = append(toolGroupList, pages.MCPSettingsToolGroup{Name: g, Tools: ts})
			}
		}
		if others := toolsByGroup["Other"]; len(others) > 0 {
			toolGroupList = append(toolGroupList, pages.MCPSettingsToolGroup{Name: "Other", Tools: others})
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

		settings := pages.MCPSettingsProps{
			CSRFToken:               GetCSRFToken(r),
			Instructions:            instructions,
			DefaultInstructions:     breadboxmcp.DefaultInstructions,
			ReviewGuidelines:        reviewGuidelines,
			DefaultReviewGuidelines: breadboxmcp.DefaultReviewGuidelines,
			ReportFormat:            reportFormat,
			DefaultReportFormat:     breadboxmcp.DefaultReportFormat,
			ToolGroups:              toolGroupList,
			ToolsEnabledCount:       enabledCount,
			ToolsDisabledCount:      totalTools - enabledCount,
			ToolsTotalCount:         totalTools,
		}

		// === Activity tab data ===
		var sessionRows []pages.AgentsSessionRow
		var sessionsPage, totalPages int
		if tab == pages.AgentsTabActivity {
			sessionsPage = parsePage(r)
			sessions, total, _ := svc.ListMCPSessions(ctx, sessionsPage, 25)
			sessionRows = make([]pages.AgentsSessionRow, 0, len(sessions))
			for _, s := range sessions {
				sessionRows = append(sessionRows, pages.AgentsSessionFromService(s, relativeTimeFromRFC3339))
			}
			totalPages = int((total + 24) / 25)
			if totalPages < 1 {
				totalPages = 1
			}
		}

		props := pages.AgentsProps{
			Tab:                tab,
			Guide:              guide,
			Wizard:             wizard,
			Settings:           settings,
			Sessions:           sessionRows,
			SessionsPage:       sessionsPage,
			SessionsTotalPages: totalPages,
		}
		renderAgents(w, r, tr, data, props)
	}
}

// renderAgents mirrors the renderPromptBuilder pattern: it hands typed
// AgentsProps to pages.Agents and uses RenderWithTempl to host it
// inside base.html.
func renderAgents(w http.ResponseWriter, r *http.Request, tr *TemplateRenderer, data map[string]any, props pages.AgentsProps) {
	tr.RenderWithTempl(w, r, data, pages.Agents(props))
}
