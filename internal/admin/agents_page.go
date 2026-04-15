package admin

import (
	"net/http"
	"strconv"

	breadboxmcp "breadbox/internal/mcp"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
)

// AgentsPageHandler serves GET /admin/agents — combined Getting Started + Agent Wizard + MCP Settings.
func AgentsPageHandler(svc *service.Service, mcpServer *breadboxmcp.MCPServer, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tab := r.URL.Query().Get("tab")
		if tab != "wizard" && tab != "settings" && tab != "activity" {
			tab = "guide"
		}

		data := BaseTemplateData(r, sm, "agents", "Agents")
		data["Tab"] = tab

		// === Getting Started data ===
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

		var pendingReviews, uncategorizedCount, ruleCount int64
		if n, err := pendingReviewsCount(ctx, svc); err == nil {
			pendingReviews = n
		}
		if cnt, err := svc.CountUncategorizedTransactions(ctx); err == nil {
			uncategorizedCount = cnt
		}

		enabled := true
		if result, err := svc.ListTransactionRules(ctx, service.TransactionRuleListParams{Enabled: &enabled, Limit: 1}); err == nil {
			ruleCount = int64(result.Total)
		}
		data["PendingReviews"] = pendingReviews
		data["UncategorizedCount"] = uncategorizedCount
		data["RuleCount"] = ruleCount

		// === Agent Wizard data ===
		var totalAccounts int64
		if accounts, err := svc.ListAccounts(ctx, nil); err == nil {
			totalAccounts = int64(len(accounts))
		}
		data["Stats"] = AgentWizardStats{
			PendingReviews: pendingReviews,
			TotalRules:     ruleCount,
			TotalAccounts:  totalAccounts,
		}

		// === MCP Settings data ===
		cfg, err := svc.GetMCPConfig(ctx)
		if err != nil {
			http.Error(w, "Failed to load MCP config", http.StatusInternalServerError)
			return
		}

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
			"pending_reviews_overview":      "Reviews",
			"list_pending_reviews":          "Reviews",
			"submit_review":                 "Reviews",
			"batch_submit_reviews":          "Reviews",
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
			"Reviews", "Rules", "Account Links", "Comments & Reports",
		}

		toolsByGroup := make(map[string][]toolInfo)
		for _, td := range mcpServer.AllToolDefs() {
			isEnabled := !disabledSet[td.Tool.Name]
			group := toolGroups[td.Tool.Name]
			if group == "" {
				group = "Other"
			}
			toolsByGroup[group] = append(toolsByGroup[group], toolInfo{
				Name:           td.Tool.Name,
				Description:    td.Tool.Description,
				Classification: string(td.Classification),
				Enabled:        isEnabled,
				Group:          group,
			})
		}
		var tools []toolInfo
		for _, g := range groupOrder {
			tools = append(tools, toolsByGroup[g]...)
		}
		if others := toolsByGroup["Other"]; len(others) > 0 {
			tools = append(tools, others...)
		}

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

		reviewGuidelines := cfg.ReviewGuidelines
		if reviewGuidelines == "" {
			reviewGuidelines = breadboxmcp.DefaultReviewGuidelines
		}
		reportFormat := cfg.ReportFormat
		if reportFormat == "" {
			reportFormat = breadboxmcp.DefaultReportFormat
		}

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

		// === Activity tab data ===
		if tab == "activity" {
			page := 1
			if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
				page = p
			}
			sessions, total, _ := svc.ListMCPSessions(ctx, page, 25)
			data["Sessions"] = sessions
			data["SessionsTotal"] = int(total)
			data["SessionsPage"] = page
			totalPages := int((total + 24) / 25)
			if totalPages < 1 {
				totalPages = 1
			}
			data["SessionsTotalPages"] = totalPages
		}

		tr.Render(w, r, "agents.html", data)
	}
}
