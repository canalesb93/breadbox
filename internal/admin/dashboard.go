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

		// Spending by category (last 30 days).
		categorySummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy: "category",
		})
		if err != nil {
			a.Logger.Error("category summary", "error", err)
		}
		var categoryLabelsJSON, categoryAmountsJSON template.JS
		if categorySummary != nil && len(categorySummary.Summary) > 0 {
			labels := make([]string, 0, len(categorySummary.Summary))
			amounts := make([]float64, 0, len(categorySummary.Summary))
			for _, row := range categorySummary.Summary {
				label := "Uncategorized"
				if row.Category != nil && *row.Category != "" {
					label = *row.Category
				}
				// Only include positive amounts (spending)
				if row.TotalAmount > 0 {
					labels = append(labels, label)
					amounts = append(amounts, row.TotalAmount)
				}
			}
			if lb, err := json.Marshal(labels); err == nil {
				categoryLabelsJSON = template.JS(lb)
			}
			if ab, err := json.Marshal(amounts); err == nil {
				categoryAmountsJSON = template.JS(ab)
			}
		}

		// Daily spending trend (last 30 days).
		dailySummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy: "day",
		})
		if err != nil {
			a.Logger.Error("daily summary", "error", err)
		}
		var dailyLabelsJSON, dailyAmountsJSON template.JS
		if dailySummary != nil && len(dailySummary.Summary) > 0 {
			// Reverse so oldest is first (API returns DESC).
			rows := dailySummary.Summary
			labels := make([]string, 0, len(rows))
			amounts := make([]float64, 0, len(rows))
			for i := len(rows) - 1; i >= 0; i-- {
				row := rows[i]
				label := ""
				if row.Period != nil {
					label = *row.Period
				}
				labels = append(labels, label)
				// Use absolute value for spending trend
				amt := row.TotalAmount
				if amt < 0 {
					amt = -amt
				}
				amounts = append(amounts, amt)
			}
			if lb, err := json.Marshal(labels); err == nil {
				dailyLabelsJSON = template.JS(lb)
			}
			if ab, err := json.Marshal(amounts); err == nil {
				dailyAmountsJSON = template.JS(ab)
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

		// Total spending (30 days).
		var totalSpending float64
		if categorySummary != nil && categorySummary.Totals.TotalAmount != nil {
			totalSpending = *categorySummary.Totals.TotalAmount
		}

		// Total income (30 days) — negative amounts in our system are credits/income.
		var totalIncome float64
		if dailySummary != nil {
			for _, row := range dailySummary.Summary {
				if row.TotalAmount < 0 {
					totalIncome += -row.TotalAmount
				}
			}
		}

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
			"DailyLabels":            dailyLabelsJSON,
			"DailyAmounts":           dailyAmountsJSON,
			"RecentTransactions":     recentTransactions,
			"TotalSpending":          totalSpending,
			"TotalIncome":            totalIncome,
			"Accounts":              dashAccounts,
			"NetWorth":              netWorth,
			"TotalAssets":           totalAssets,
			"TotalLiabilities":     totalLiabilities,
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
