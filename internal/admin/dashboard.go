package admin

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"sort"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/db"
	"breadbox/internal/service"

	"github.com/jackc/pgx/v5/pgtype"
)

// DashboardHandler serves GET /admin/ — the dashboard home page.
func DashboardHandler(a *app.App, svc *service.Service, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Parse chart date range from query param (default 30 days).
		chartDays := 30
		if d := r.URL.Query().Get("days"); d != "" {
			switch d {
			case "7":
				chartDays = 7
			case "30":
				chartDays = 30
			case "90":
				chartDays = 90
			case "365":
				chartDays = 365
			}
		}
		chartStart := time.Now().AddDate(0, 0, -chartDays)

		accountCount, err := a.Queries.CountAccounts(ctx)
		if err != nil {
			a.Logger.Error("count accounts", "error", err)
			accountCount = 0
		}

		txCount, err := a.Queries.CountTransactions(ctx)
		if err != nil {
			a.Logger.Error("count transactions", "error", err)
			txCount = 0
		}

		lastSync, err := a.Queries.GetLastSuccessfulSyncTime(ctx)
		if err != nil {
			a.Logger.Error("get last sync time", "error", err)
		}

		needsAttention, err := a.Queries.CountConnectionsNeedingAttention(ctx)
		if err != nil {
			a.Logger.Error("count connections needing attention", "error", err)
			needsAttention = 0
		}

		reviewPending, err := a.Queries.CountPendingReviews(ctx)
		if err != nil {
			a.Logger.Error("count pending reviews", "error", err)
			reviewPending = 0
		}

		recentLogs, err := a.Queries.ListRecentSyncLogs(ctx)
		if err != nil {
			a.Logger.Error("list recent sync logs", "error", err)
		}

		lastSyncText := "Never"
		if lastSync.Valid {
			lastSyncText = relativeTime(lastSync.Time)
		}

		// Spending by category for the selected date range — only positive amounts (debits).
		categorySummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "category",
			StartDate:    &chartStart,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("category summary", "error", err)
		}
		var categoryLabelsJSON, categoryAmountsJSON, categoryColorsJSON template.JS
		// Top spending categories for the breakdown list.
		type CategorySpend struct {
			Name    string
			Color   string
			Amount  float64
			Percent float64
		}
		// Curated fallback palette for categories without a DB color.
		categoryPalette := []string{
			"oklch(0.55 0.12 250)", // blue
			"oklch(0.55 0.14 160)", // teal
			"oklch(0.58 0.12 35)",  // warm amber
			"oklch(0.52 0.14 300)", // purple
			"oklch(0.58 0.10 80)",  // olive
			"oklch(0.50 0.12 200)", // slate blue
			"oklch(0.55 0.12 120)", // green
			"oklch(0.48 0.10 350)", // rose
			"oklch(0.45 0 0)",      // gray (for "Other")
		}

		var topCategories []CategorySpend
		var maxCategorySpend float64
		if categorySummary != nil && len(categorySummary.Summary) > 0 {
			labels := make([]string, 0, len(categorySummary.Summary))
			amounts := make([]float64, 0, len(categorySummary.Summary))
			colors := make([]string, 0, len(categorySummary.Summary))
			for i, row := range categorySummary.Summary {
				label := "Uncategorized"
				if row.Category != nil && *row.Category != "" {
					label = *row.Category
				}
				labels = append(labels, label)
				amounts = append(amounts, row.TotalAmount)
				// Use DB color if set, otherwise use palette color.
				color := ""
				if row.CategoryColor != nil && *row.CategoryColor != "" {
					color = *row.CategoryColor
				} else {
					color = categoryPalette[i%len(categoryPalette)]
				}
				colors = append(colors, color)
				topCategories = append(topCategories, CategorySpend{Name: label, Color: color, Amount: row.TotalAmount})
				if row.TotalAmount > maxCategorySpend {
					maxCategorySpend = row.TotalAmount
				}
			}
			if lb, err := json.Marshal(labels); err == nil {
				categoryLabelsJSON = template.JS(lb)
			}
			if ab, err := json.Marshal(amounts); err == nil {
				categoryAmountsJSON = template.JS(ab)
			}
			if cb, err := json.Marshal(colors); err == nil {
				categoryColorsJSON = template.JS(cb)
			}
		}
		// Limit to top 8 categories for the bar chart.
		if len(topCategories) > 8 {
			topCategories = topCategories[:8]
		}
		// Calculate percentages for horizontal bar chart.
		if catTotal := func() float64 {
			var t float64
			for _, c := range topCategories {
				t += c.Amount
			}
			return t
		}(); catTotal > 0 {
			for i := range topCategories {
				topCategories[i].Percent = (topCategories[i].Amount / catTotal) * 100
			}
		}

		// Daily spending trend for selected range — only positive amounts (debits).
		// For 365-day range, group by month instead of day.
		chartGroupBy := "day"
		if chartDays == 365 {
			chartGroupBy = "month"
		}
		dailySummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      chartGroupBy,
			StartDate:    &chartStart,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("daily summary", "error", err)
		}

		// Daily income for the same period (for chart overlay).
		dailyIncomeSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:   chartGroupBy,
			StartDate: &chartStart,
		})
		if err != nil {
			a.Logger.Error("daily income summary", "error", err)
		}
		// Build income map (period -> abs(negative amounts)).
		incomeByPeriod := make(map[string]float64)
		if dailyIncomeSummary != nil {
			for _, row := range dailyIncomeSummary.Summary {
				if row.TotalAmount < 0 && row.Period != nil {
					incomeByPeriod[*row.Period] = -row.TotalAmount
				}
			}
		}

		// Previous period spending for comparison.
		prevStart := time.Now().AddDate(0, 0, -chartDays*2)
		prevEnd := time.Now().AddDate(0, 0, -chartDays)
		prevSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "category",
			StartDate:    &prevStart,
			EndDate:      &prevEnd,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("previous period summary", "error", err)
		}
		var prevTotalSpending float64
		if prevSummary != nil {
			for _, row := range prevSummary.Summary {
				prevTotalSpending += row.TotalAmount
			}
		}
		// Calculate spending change percentage.
		var spendingChangePercent float64
		var hasSpendingChange bool
		// We compute these after totalSpending is calculated below.
		var dailyLabelsJSON, dailyAmountsJSON, dailyIncomeAmountsJSON template.JS
		if dailySummary != nil && len(dailySummary.Summary) > 0 {
			// Reverse so oldest is first (API returns DESC).
			rows := dailySummary.Summary
			labels := make([]string, 0, len(rows))
			amounts := make([]float64, 0, len(rows))
			incomeAmounts := make([]float64, 0, len(rows))
			for i := len(rows) - 1; i >= 0; i-- {
				row := rows[i]
				label := ""
				if row.Period != nil {
					label = *row.Period
				}
				labels = append(labels, label)
				amounts = append(amounts, row.TotalAmount)
				incomeAmounts = append(incomeAmounts, incomeByPeriod[label])
			}
			if lb, err := json.Marshal(labels); err == nil {
				dailyLabelsJSON = template.JS(lb)
			}
			if ab, err := json.Marshal(amounts); err == nil {
				dailyAmountsJSON = template.JS(ab)
			}
			if ib, err := json.Marshal(incomeAmounts); err == nil {
				dailyIncomeAmountsJSON = template.JS(ib)
			}
		}

		// Recent transactions (last 10).
		recentTx, err := svc.ListTransactionsAdmin(ctx, service.AdminTransactionListParams{
			Page:     1,
			PageSize: 8,
		})
		if err != nil {
			a.Logger.Error("recent transactions", "error", err)
		}

		// Onboarding checklist detection.
		showOnboarding := false
		var hasProvider, hasMember, hasConnection bool

		dismissed, _ := a.Queries.GetAppConfig(ctx, "onboarding_dismissed")
		if !(dismissed.Value.Valid && dismissed.Value.String == "true") {
			showOnboarding = true

			// Check provider
			hasProvider = a.Config.PlaidClientID != "" || a.Config.TellerAppID != ""

			// Check members
			userCount, err := a.Queries.CountUsers(ctx)
			if err != nil {
				a.Logger.Error("count users", "error", err)
			}
			hasMember = userCount > 0

			// Check connections
			connCount, err := a.Queries.CountConnections(ctx)
			if err != nil {
				a.Logger.Error("count connections", "error", err)
			}
			hasConnection = connCount > 0
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
				dismissed, _ := a.Queries.GetAppConfig(ctx, "update_dismissed_version")
				if !(dismissed.Value.Valid && dismissed.Value.String == latest.Version) {
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

		// Total spending for the selected period.
		var totalSpending float64
		if categorySummary != nil && categorySummary.Totals.TotalAmount != nil {
			totalSpending = *categorySummary.Totals.TotalAmount
		}

		// Compute spending change vs previous period.
		if prevTotalSpending > 0 {
			hasSpendingChange = true
			spendingChangePercent = ((totalSpending - prevTotalSpending) / prevTotalSpending) * 100
		}

		// Total income for the selected date range — negative amounts in our system are credits/income.
		// Use a separate summary query without SpendingOnly to get income totals.
		var totalIncome float64
		incomeSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:   "day",
			StartDate: &chartStart,
		})
		if err != nil {
			a.Logger.Error("income summary", "error", err)
		}
		if incomeSummary != nil {
			for _, row := range incomeSummary.Summary {
				if row.TotalAmount < 0 {
					totalIncome += -row.TotalAmount
				}
			}
		}

		// Cash flow: net = income - spending, savings rate = net/income * 100.
		var cashFlowNet float64
		var savingsRate float64
		var hasCashFlow bool
		if totalIncome > 0 || totalSpending > 0 {
			hasCashFlow = true
			cashFlowNet = totalIncome - totalSpending
			if totalIncome > 0 {
				savingsRate = (cashFlowNet / totalIncome) * 100
			}
		}

		// Spending vs income ratio for the visual bar (spending as % of income).
		var spendingRatio float64
		if totalIncome > 0 {
			spendingRatio = (totalSpending / totalIncome) * 100
			if spendingRatio > 100 {
				spendingRatio = 100
			}
		}

		// ── Spending Pace: current month vs last month ──────────────────────
		// Always computed regardless of date picker selection.
		today := time.Now()
		monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
		daysElapsed := today.Day()
		daysInMonth := time.Date(today.Year(), today.Month()+1, 0, 0, 0, 0, 0, today.Location()).Day()
		daysRemaining := daysInMonth - daysElapsed

		// Current month spending (1st → today).
		var currentMonthSpending float64
		currentMonthSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "day",
			StartDate:    &monthStart,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("current month spending", "error", err)
		}
		if currentMonthSummary != nil {
			for _, row := range currentMonthSummary.Summary {
				currentMonthSpending += row.TotalAmount
			}
		}

		// Current month income.
		var currentMonthIncome float64
		currentMonthIncomeSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:   "day",
			StartDate: &monthStart,
		})
		if err != nil {
			a.Logger.Error("current month income", "error", err)
		}
		if currentMonthIncomeSummary != nil {
			for _, row := range currentMonthIncomeSummary.Summary {
				if row.TotalAmount < 0 {
					currentMonthIncome += -row.TotalAmount
				}
			}
		}

		// Last month total spending.
		lastMonthStart := time.Date(today.Year(), today.Month()-1, 1, 0, 0, 0, 0, today.Location())
		lastMonthEnd := monthStart // first of current month
		var lastMonthSpending float64
		lastMonthSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "day",
			StartDate:    &lastMonthStart,
			EndDate:      &lastMonthEnd,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("last month spending", "error", err)
		}
		if lastMonthSummary != nil {
			for _, row := range lastMonthSummary.Summary {
				lastMonthSpending += row.TotalAmount
			}
		}

		// Last month spending at the same point (1st → same day of last month).
		lastMonthSameDay := time.Date(today.Year(), today.Month()-1, 1, 0, 0, 0, 0, today.Location())
		lastMonthDaysInMonth := time.Date(lastMonthSameDay.Year(), lastMonthSameDay.Month()+1, 0, 0, 0, 0, 0, today.Location()).Day()
		sameDayOfLastMonth := daysElapsed
		if sameDayOfLastMonth > lastMonthDaysInMonth {
			sameDayOfLastMonth = lastMonthDaysInMonth
		}
		lastMonthSameDayEnd := time.Date(lastMonthSameDay.Year(), lastMonthSameDay.Month(), sameDayOfLastMonth+1, 0, 0, 0, 0, today.Location())
		var lastMonthPaceSpending float64
		lastMonthPaceSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "day",
			StartDate:    &lastMonthSameDay,
			EndDate:      &lastMonthSameDayEnd,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("last month pace spending", "error", err)
		}
		if lastMonthPaceSummary != nil {
			for _, row := range lastMonthPaceSummary.Summary {
				lastMonthPaceSpending += row.TotalAmount
			}
		}

		// Compute pace metrics.
		var dailyAvgSpending float64
		var projectedMonthly float64
		var pacePercent float64    // How current month compares to last month at same point
		var hasPaceData bool
		var paceVsLastMonth string // "ahead", "behind", "same"

		if daysElapsed > 0 && currentMonthSpending > 0 {
			hasPaceData = true
			dailyAvgSpending = currentMonthSpending / float64(daysElapsed)
			projectedMonthly = dailyAvgSpending * float64(daysInMonth)

			if lastMonthPaceSpending > 0 {
				pacePercent = ((currentMonthSpending - lastMonthPaceSpending) / lastMonthPaceSpending) * 100
				if pacePercent > 2.0 {
					paceVsLastMonth = "ahead"
				} else if pacePercent < -2.0 {
					paceVsLastMonth = "behind"
				} else {
					paceVsLastMonth = "same"
				}
			}
		}

		// Progress through the month (percent).
		monthProgress := float64(daysElapsed) / float64(daysInMonth) * 100

		// Accounts with balances for the overview section.
		accounts, err := svc.ListAccounts(ctx, nil)
		if err != nil {
			a.Logger.Error("list accounts for dashboard", "error", err)
		}

		// Compute net worth and group by type.
		var netWorth float64
		var totalAssets, totalLiabilities float64
		type DashboardAccount struct {
			ID              string
			Name            string
			InstitutionName string
			Type            string
			Subtype         string
			Mask            string
			BalanceCurrent  float64
			IsoCurrencyCode string
			IsLiability     bool
			SparklineData   template.JS // JSON array of daily spending amounts (30 days)
			SpendingTotal   float64     // Total spending in last 30 days for this account
		}
		var dashAccounts []DashboardAccount
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

			isLiability := acct.Type == "credit" || acct.Type == "loan"
			if isLiability {
				totalLiabilities += math.Abs(bal)
				netWorth -= math.Abs(bal)
			} else {
				totalAssets += bal
				netWorth += bal
			}

			// Fetch per-account daily spending for sparkline.
			acctID := acct.ID
			acctDailySummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:      "day",
				AccountID:    &acctID,
				SpendingOnly: true,
			})
			var sparklineJSON template.JS
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
					sparklineJSON = template.JS(sb)
				}
			}

			dashAccounts = append(dashAccounts, DashboardAccount{
				ID:              acct.ID,
				Name:            acct.Name,
				InstitutionName: institution,
				Type:            acct.Type,
				Subtype:         subtype,
				Mask:            mask,
				BalanceCurrent:  bal,
				IsoCurrencyCode: currency,
				IsLiability:     isLiability,
				SparklineData:   sparklineJSON,
				SpendingTotal:   acctSpendTotal,
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

		// Group accounts by asset vs liability for the dashboard layout.
		type AccountGroup struct {
			Label    string // "Cash & Savings", "Investments", "Credit Cards", "Loans"
			Icon     string // Lucide icon name
			Type     string // "asset" or "liability"
			Total    float64
			Accounts []DashboardAccount
		}
		accountGroupOrder := []string{"depository", "investment", "credit", "loan"}
		accountGroupMeta := map[string]struct {
			Label string
			Icon  string
			Type  string
		}{
			"depository": {Label: "Cash & Savings", Icon: "landmark", Type: "asset"},
			"investment": {Label: "Investments", Icon: "trending-up", Type: "asset"},
			"credit":     {Label: "Credit Cards", Icon: "credit-card", Type: "liability"},
			"loan":       {Label: "Loans", Icon: "banknote", Type: "liability"},
		}
		groupMap := make(map[string]*AccountGroup)
		for _, acct := range dashAccounts {
			key := acct.Type
			if _, ok := accountGroupMeta[key]; !ok {
				key = "depository" // fallback for unknown types
			}
			if g, ok := groupMap[key]; ok {
				g.Accounts = append(g.Accounts, acct)
				if acct.IsLiability {
					g.Total += math.Abs(acct.BalanceCurrent)
				} else {
					g.Total += acct.BalanceCurrent
				}
			} else {
				meta := accountGroupMeta[key]
				bal := acct.BalanceCurrent
				if acct.IsLiability {
					bal = math.Abs(bal)
				}
				groupMap[key] = &AccountGroup{
					Label:    meta.Label,
					Icon:     meta.Icon,
					Type:     meta.Type,
					Total:    bal,
					Accounts: []DashboardAccount{acct},
				}
			}
		}
		var assetGroups, liabilityGroups []AccountGroup
		for _, key := range accountGroupOrder {
			if g, ok := groupMap[key]; ok {
				if g.Type == "asset" {
					assetGroups = append(assetGroups, *g)
				} else {
					liabilityGroups = append(liabilityGroups, *g)
				}
			}
		}

		// Allocation bar: proportion of total assets in each asset type.
		type AllocationSlice struct {
			Label   string
			Amount  float64
			Percent float64
			Color   string // OKLCH color for the bar segment
		}
		allocationColors := map[string]string{
			"depository": "oklch(0.55 0.14 155)", // green
			"investment": "oklch(0.55 0.12 250)", // blue
			"credit":     "oklch(0.58 0.12 35)",  // amber
			"loan":       "oklch(0.55 0.15 25)",  // red-ish
		}
		var allocationSlices []AllocationSlice
		grossTotal := totalAssets + totalLiabilities
		if grossTotal > 0 {
			for _, key := range accountGroupOrder {
				if g, ok := groupMap[key]; ok {
					pct := (g.Total / grossTotal) * 100
					if pct < 0.5 {
						continue // skip tiny slices
					}
					color := allocationColors[key]
					if color == "" {
						color = "oklch(0.45 0 0)"
					}
					allocationSlices = append(allocationSlices, AllocationSlice{
						Label:   g.Label,
						Amount:  g.Total,
						Percent: pct,
						Color:   color,
					})
				}
			}
		}

		// Net worth trend: compute daily net worth by working backwards from current balance.
		// Query all daily transaction totals (net: spending positive, income negative) for chart period.
		netWorthTrendStart := time.Now().AddDate(0, 0, -chartDays)
		dailyNetSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:   "day",
			StartDate: &netWorthTrendStart,
		})
		if err != nil {
			a.Logger.Error("daily net summary for net worth trend", "error", err)
		}
		// Build map of date -> net amount (positive = money out, negative = money in).
		dailyNetMap := make(map[string]float64)
		if dailyNetSummary != nil {
			for _, row := range dailyNetSummary.Summary {
				if row.Period != nil {
					dailyNetMap[*row.Period] = row.TotalAmount
				}
			}
		}
		// Build net worth series: start from today's net worth, walk backwards.
		now := time.Now()
		nwDays := chartDays
		if nwDays > 90 {
			nwDays = 90 // Cap at 90 days for readability
		}
		nwLabels := make([]string, nwDays+1)
		nwValues := make([]float64, nwDays+1)
		nwValues[nwDays] = netWorth
		nwLabels[nwDays] = now.Format("2006-01-02")
		for i := nwDays - 1; i >= 0; i-- {
			day := now.AddDate(0, 0, -(nwDays - i))
			dayStr := day.Format("2006-01-02")
			nwLabels[i] = dayStr
			// Net worth on day[i] = net worth on day[i+1] - net_transactions on day[i+1].
			nextDayStr := now.AddDate(0, 0, -(nwDays - i - 1)).Format("2006-01-02")
			nwValues[i] = nwValues[i+1] - dailyNetMap[nextDayStr]
		}
		var netWorthLabelsJSON, netWorthValuesJSON template.JS
		if lb, err := json.Marshal(nwLabels); err == nil {
			netWorthLabelsJSON = template.JS(lb)
		}
		if vb, err := json.Marshal(nwValues); err == nil {
			netWorthValuesJSON = template.JS(vb)
		}

		data := map[string]any{
			"PageTitle":              "Dashboard",
			"CurrentPage":            "dashboard",
			"AccountCount":           accountCount,
			"TxCount":                txCount,
			"LastSync":               lastSyncText,
			"NeedsAttention":         needsAttention,
			"RecentLogs":             recentLogs,
			"CSRFToken":              GetCSRFToken(r),
			"ShowOnboarding":         showOnboarding,
			"HasProvider":            hasProvider,
			"HasMember":              hasMember,
			"HasConnection":          hasConnection,
			"ShowUpdateBanner":       showUpdateBanner,
			"LatestVersion":          latestVersion,
			"LatestURL":              latestURL,
			"CurrentVersion":         currentVersion,
			"DockerSocketAvailable":  a.DockerSocketAvailable,
			"ReviewPending":          reviewPending,
			"CategoryLabels":         categoryLabelsJSON,
			"CategoryAmounts":        categoryAmountsJSON,
			"CategoryColors":         categoryColorsJSON,
			"DailyLabels":            dailyLabelsJSON,
			"DailyAmounts":           dailyAmountsJSON,
			"DailyIncomeAmounts":     dailyIncomeAmountsJSON,
			"ChartDays":              chartDays,
			"RecentTransactions":     recentTransactions,
			"TotalSpending":          totalSpending,
			"TotalIncome":            totalIncome,
			"Accounts":               dashAccounts,
			"NetWorth":               netWorth,
			"TotalAssets":            totalAssets,
			"TotalLiabilities":       totalLiabilities,
			"TopCategories":          topCategories,
			"MaxCategorySpend":       maxCategorySpend,
			"PrevTotalSpending":      prevTotalSpending,
			"SpendingChangePercent":  spendingChangePercent,
			"HasSpendingChange":      hasSpendingChange,
			"NetWorthLabels":         netWorthLabelsJSON,
			"NetWorthValues":         netWorthValuesJSON,
			"CashFlowNet":            cashFlowNet,
			"SavingsRate":            savingsRate,
			"HasCashFlow":            hasCashFlow,
			"SpendingRatio":          spendingRatio,
			// Spending pace data.
			"HasPaceData":            hasPaceData,
			"CurrentMonthSpending":   currentMonthSpending,
			"CurrentMonthIncome":     currentMonthIncome,
			"LastMonthSpending":      lastMonthSpending,
			"LastMonthPaceSpending":  lastMonthPaceSpending,
			"DailyAvgSpending":       dailyAvgSpending,
			"ProjectedMonthly":       projectedMonthly,
			"PacePercent":            pacePercent,
			"PaceVsLastMonth":        paceVsLastMonth,
			"DaysElapsed":            daysElapsed,
			"DaysInMonth":            daysInMonth,
			"DaysRemaining":          daysRemaining,
			"MonthProgress":          monthProgress,
			"CurrentMonthName":       today.Format("January"),
			"LastMonthName":          lastMonthStart.Format("January"),
			// Grouped accounts.
			"AssetGroups":            assetGroups,
			"LiabilityGroups":       liabilityGroups,
			"AllocationSlices":       allocationSlices,
		}
		tr.Render(w, r, "dashboard.html", data)
	}
}

// DismissOnboardingHandler handles POST /admin/onboarding/dismiss.
func DismissOnboardingHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = a.Queries.SetAppConfig(r.Context(), db.SetAppConfigParams{
			Key:   "onboarding_dismissed",
			Value: pgtype.Text{String: "true", Valid: true},
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
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
