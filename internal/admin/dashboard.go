package admin

import (
	"fmt"
	"net/http"
	"time"

	"breadbox/internal/app"
)

// DashboardHandler serves GET /admin/ — the dashboard home page.
func DashboardHandler(a *app.App, tr *TemplateRenderer) http.HandlerFunc {
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

		recentLogs, err := a.Queries.ListRecentSyncLogs(ctx)
		if err != nil {
			a.Logger.Error("list recent sync logs", "error", err)
		}

		lastSyncText := "Never"
		if lastSync.Valid {
			lastSyncText = relativeTime(lastSync.Time)
		}

		data := map[string]any{
			"PageTitle":      "Dashboard",
			"CurrentPage":    "dashboard",
			"AccountCount":   accountCount,
			"TxCount":        txCount,
			"LastSync":       lastSyncText,
			"NeedsAttention": needsAttention,
			"RecentLogs":     recentLogs,
			"CSRFToken":      GetCSRFToken(r),
		}
		tr.Render(w, r, "dashboard.html", data)
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
