package admin

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/appconfig"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"
)

// DashboardHandler serves GET /admin/ — the dashboard home page.
// When onboarding has not been dismissed, the root path redirects to /getting-started.
func DashboardHandler(a *app.App, svc *service.Service, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Redirect to getting-started page if onboarding is not dismissed.
		if !appconfig.Bool(ctx, a.Queries, "onboarding_dismissed", false) {
			http.Redirect(w, r, "/getting-started", http.StatusSeeOther)
			return
		}

		needsAttention, err := a.Queries.CountConnectionsNeedingAttention(ctx)
		if err != nil {
			a.Logger.Error("count connections needing attention", "error", err)
			needsAttention = 0
		}

		// Review queue is backed by the needs-review tag.
		reviewsEnabled := true
		reviewPending, err := a.Queries.CountTransactionsWithTagSlug(ctx, "needs-review")
		if err != nil {
			a.Logger.Error("count pending reviews", "error", err)
			reviewPending = 0
		}

		// Recent transactions (last 8).
		recentTx, err := svc.ListTransactionsAdmin(ctx, service.AdminTransactionListParams{
			Page:     1,
			PageSize: 8,
		})
		if err != nil {
			a.Logger.Error("recent transactions", "error", err)
		}

		// Version check for update banner.
		var showUpdateBanner bool
		var latestVersion, latestURL string
		currentVersion := a.Config.Version

		if currentVersion != "dev" && a.VersionChecker != nil {
			updateAvailable, latest, err := a.VersionChecker.CheckForUpdate(ctx)
			if err != nil {
				a.Logger.Debug("version check failed", "error", err)
			}
			if updateAvailable != nil && *updateAvailable && latest != nil {
				if appconfig.String(ctx, a.Queries, "update_dismissed_version", "") != latest.Version {
					showUpdateBanner = true
					latestVersion = latest.Version
					latestURL = latest.URL
				}
			}
		}

		// Build recent transactions data for template.
		var recentTransactions []service.AdminTransactionRow
		if recentTx != nil {
			recentTransactions = recentTx.Transactions
		}

		// Total transaction count for the overview stats.
		totalTransactions, err := svc.CountTransactions(ctx)
		if err != nil {
			a.Logger.Error("count transactions for dashboard", "error", err)
		}

		// Accounts with balances for the overview section.
		accounts, err := svc.ListAccounts(ctx, nil)
		if err != nil {
			a.Logger.Error("list accounts for dashboard", "error", err)
		}

		// Group accounts by type for display. DashboardAccount lives in
		// components/pages so the templ dashboard component can consume it
		// without a back-import into admin.
		var dashAccounts []pages.DashboardAccount
		for _, acct := range accounts {
			if acct.BalanceCurrent == nil {
				continue
			}
			bal := *acct.BalanceCurrent
			institution := ""
			if acct.InstitutionName != nil {
				institution = *acct.InstitutionName
			}
			subtype := ""
			if acct.Subtype != nil {
				subtype = *acct.Subtype
			}
			mask := ""
			if acct.Mask != nil {
				mask = *acct.Mask
			}
			currency := "USD"
			if acct.IsoCurrencyCode != nil {
				currency = *acct.IsoCurrencyCode
			}

			isLiability := IsLiabilityAccount(acct.Type)

			// Fetch per-account daily spending for sparkline.
			acctID := acct.ID
			acctDailySummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:      "day",
				AccountID:    &acctID,
				SpendingOnly: true,
			})
			var sparklineJSON string
			var acctSpendTotal float64
			if err == nil && acctDailySummary != nil && len(acctDailySummary.Summary) > 0 {
				// Build a map of date->amount, then fill in 30 days.
				dayMap := make(map[string]float64, len(acctDailySummary.Summary))
				for _, row := range acctDailySummary.Summary {
					if row.Period != nil {
						dayMap[*row.Period] = row.TotalAmount
						acctSpendTotal += row.TotalAmount
					}
				}
				// Build array for last 30 days (oldest first).
				now := time.Now()
				sparkData := make([]float64, 30)
				for i := 29; i >= 0; i-- {
					day := now.AddDate(0, 0, -i).Format("2006-01-02")
					sparkData[29-i] = dayMap[day]
				}
				if sb, err := json.Marshal(sparkData); err == nil {
					sparklineJSON = string(sb)
				}
			}

			connStatus := ""
			if acct.ConnectionStatus != nil {
				connStatus = *acct.ConnectionStatus
			}
			dashAccounts = append(dashAccounts, pages.DashboardAccount{
				ID:               acct.ID,
				Name:             acct.Name,
				InstitutionName:  institution,
				Type:             acct.Type,
				Subtype:          subtype,
				Mask:             mask,
				BalanceCurrent:   bal,
				IsoCurrencyCode:  currency,
				IsLiability:      isLiability,
				SparklineData:    sparklineJSON,
				SpendingTotal:    acctSpendTotal,
				ConnectionStatus: connStatus,
			})
		}
		// Sort: depository first, then credit, then loan, then others.
		typeOrder := map[string]int{"depository": 0, "investment": 1, "credit": 2, "loan": 3}
		sort.Slice(dashAccounts, func(i, j int) bool {
			oi, oj := 4, 4
			if v, ok := typeOrder[dashAccounts[i].Type]; ok {
				oi = v
			}
			if v, ok := typeOrder[dashAccounts[j].Type]; ok {
				oj = v
			}
			if oi != oj {
				return oi < oj
			}
			return dashAccounts[i].Name < dashAccounts[j].Name
		})

		// Compute asset/liability totals for allocation bar.
		var totalAssets, totalLiabilities float64
		for _, acct := range dashAccounts {
			if acct.IsLiability {
				totalLiabilities += math.Abs(acct.BalanceCurrent)
			} else {
				totalAssets += acct.BalanceCurrent
			}
		}

		// Allocation bar: group totals by type for the proportion bar.
		accountGroupOrder := []string{"depository", "investment", "credit", "loan"}
		accountGroupLabels := map[string]string{
			"depository": "Cash & Savings",
			"investment": "Investments",
			"credit":     "Credit Cards",
			"loan":       "Loans",
		}
		// Build per-type totals for allocation bar.
		typeTotals := make(map[string]float64)
		for _, acct := range dashAccounts {
			key := acct.Type
			if _, ok := accountGroupLabels[key]; !ok {
				key = "depository"
			}
			if acct.IsLiability {
				typeTotals[key] += math.Abs(acct.BalanceCurrent)
			} else {
				typeTotals[key] += acct.BalanceCurrent
			}
		}

		// Allocation bar: proportion of total assets in each asset type.
		// AllocationSlice lives in components/pages.
		allocationColors := map[string]string{
			"depository": "oklch(0.55 0.14 155)", // green
			"investment": "oklch(0.55 0.12 250)", // blue
			"credit":     "oklch(0.58 0.12 35)",  // amber
			"loan":       "oklch(0.55 0.15 25)",  // red-ish
		}
		var allocationSlices []pages.AllocationSlice
		grossTotal := totalAssets + totalLiabilities
		if grossTotal > 0 {
			for _, key := range accountGroupOrder {
				total, ok := typeTotals[key]
				if !ok || total <= 0 {
					continue
				}
				pct := (total / grossTotal) * 100
				if pct < 0.5 {
					continue // skip tiny slices
				}
				color := allocationColors[key]
				if color == "" {
					color = "oklch(0.45 0 0)"
				}
				label := accountGroupLabels[key]
				allocationSlices = append(allocationSlices, pages.AllocationSlice{
					Label:   label,
					Amount:  total,
					Percent: pct,
					Color:   color,
				})
			}
		}

		// ── Connection Health: per-connection status for dashboard panel ──
		// ConnectionHealthRow lives in components/pages. We keep a local
		// wrapper with lastSyncedRaw so we can sort by sync time before
		// dropping to the exported shape.
		type connectionHealthRow struct {
			pages.ConnectionHealthRow
			lastSyncedRaw time.Time
		}
		var connectionHealth []connectionHealthRow
		var healthyCount, errorCount, staleCount int

		bankConnections, err := a.Queries.ListBankConnections(ctx)
		if err != nil {
			a.Logger.Error("list bank connections for health", "error", err)
		}

		for _, conn := range bankConnections {
			if string(conn.Status) == "disconnected" {
				continue
			}
			name := "Unknown"
			if conn.InstitutionName.Valid {
				name = conn.InstitutionName.String
			}
			errMsg := ""
			if conn.ErrorMessage.Valid {
				errMsg = conn.ErrorMessage.String
			}
			lastSync := "Never"
			isStale := ConnectionStaleness(
				a.Config.SyncIntervalMinutes,
				conn.SyncIntervalOverrideMinutes,
				conn.LastSyncedAt,
				time.Now(),
			)

			if conn.LastSyncedAt.Valid {
				lastSync = relativeTime(conn.LastSyncedAt.Time)
			}

			status := string(conn.Status)
			switch status {
			case "active":
				if isStale {
					staleCount++
				} else {
					healthyCount++
				}
			case "error", "pending_reauth":
				errorCount++
			}

			connID := pgconv.FormatUUID(conn.ID)

			var rawTime time.Time
			if conn.LastSyncedAt.Valid {
				rawTime = conn.LastSyncedAt.Time
			}
			connectionHealth = append(connectionHealth, connectionHealthRow{
				ConnectionHealthRow: pages.ConnectionHealthRow{
					ID:           connID,
					Name:         name,
					Provider:     string(conn.Provider),
					Status:       status,
					ErrorMessage: errMsg,
					LastSyncedAt: lastSync,
					AccountCount: conn.AccountCount,
					Paused:       conn.Paused,
					IsStale:      isStale,
				},
				lastSyncedRaw: rawTime,
			})
		}

		// Sort connections by last sync time (most recent first)
		sort.Slice(connectionHealth, func(i, j int) bool {
			return connectionHealth[i].lastSyncedRaw.After(connectionHealth[j].lastSyncedRaw)
		})

		// ── Sync Health Summary ──────────────────────────────────────
		syncHealth, err := svc.GetSyncHealthSummary(ctx)
		if err != nil {
			a.Logger.Error("sync health summary", "error", err)
		}
		// Enrich with connection-level errors (already computed above) and next sync time.
		if syncHealth != nil {
			syncHealth.ConnectionErrors = int64(errorCount)
			if a.Scheduler != nil {
				syncHealth.NextSyncTime = formatNextSync(a.Scheduler.NextRun())
			}
			// Override health if connections have errors.
			if errorCount > 0 && syncHealth.OverallHealth == "healthy" {
				syncHealth.OverallHealth = "degraded"
			}
		}

		// Pending reviews count for badge.
		var uncatCount int64
		err = a.DB.QueryRow(ctx, "SELECT COUNT(*) FROM transactions WHERE deleted_at IS NULL AND category_id IS NULL AND pending = false").Scan(&uncatCount)
		if err != nil {
			a.Logger.Error("count uncategorized", "error", err)
		}

		// Agent reports: load recent unread reports for the dashboard widget.
		// DashboardReport lives in components/pages.
		const dashboardReportsVisible = 8
		var agentReports []pages.DashboardReport
		rawReports, err := svc.ListUnreadAgentReports(ctx, dashboardReportsVisible)
		if err != nil {
			a.Logger.Error("list unread agent reports", "error", err)
		}
		// Reuse the unread count already fetched by NavBadgesMiddleware —
		// avoids a second COUNT(*) round-trip on every dashboard load.
		totalUnread := int(getNavBadges(ctx).UnreadReports)
		for _, r := range rawReports {
			t, _ := time.Parse(time.RFC3339, r.CreatedAt)
			agentReports = append(agentReports, pages.DashboardReport{
				ID:            r.ID,
				Title:         r.Title,
				Priority:      r.Priority,
				DisplayAuthor: reportDisplayAuthor(r.CreatedByName, r.Author),
				CreatedAt:     relativeTime(t),
			})
		}
		moreReportsCount := totalUnread - len(agentReports)

		// Compute attention items count for the dashboard card.
		attentionCount := 0
		if uncatCount > 0 {
			attentionCount++
		}
		if reviewsEnabled && reviewPending > 0 {
			attentionCount++
		}
		if errorCount > 0 {
			attentionCount++
		}

		// Drop the internal health wrapper for the exported view model —
		// the lastSyncedRaw field is only used for sorting above.
		exportedHealth := make([]pages.ConnectionHealthRow, len(connectionHealth))
		for i, h := range connectionHealth {
			exportedHealth[i] = h.ConnectionHealthRow
		}

		data := map[string]any{
			"PageTitle":   "Home",
			"CurrentPage": "home",
			"CSRFToken":   GetCSRFToken(r),
		}
		body := pages.Dashboard(pages.DashboardProps{
			CSRFToken:             GetCSRFToken(r),
			ShowUpdateBanner:      showUpdateBanner,
			LatestVersion:         latestVersion,
			LatestURL:             latestURL,
			CurrentVersion:        currentVersion,
			DockerSocketAvailable: a.DockerSocketAvailable,
			NeedsAttention:        needsAttention,
			Accounts:              dashAccounts,
			AllocationSlices:      allocationSlices,
			TotalAssets:           totalAssets,
			TotalLiabilities:      totalLiabilities,
			NetWorth:              totalAssets - totalLiabilities,
			AgentReports:          agentReports,
			TotalUnreadReports:    totalUnread,
			MoreReportsCount:      moreReportsCount,
			RecentTransactions:    recentTransactions,
			TotalTransactions:     totalTransactions,
			HasAttentionItems:     attentionCount > 0,
			AttentionCount:        attentionCount,
			UncategorizedCount:    uncatCount,
			ReviewsEnabled:        reviewsEnabled,
			ReviewPending:         reviewPending,
			ErrorCount:            errorCount,
			ConnectionHealth:      exportedHealth,
			SyncHealth:            syncHealth,
		})
		tr.RenderWithTempl(w, r, data, body)
	}
}

// relativeTime converts a time to a human-readable relative string.
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
