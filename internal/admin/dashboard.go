package admin

import (
	"fmt"
	"net/http"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
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
		http.Redirect(w, r, "/admin/", http.StatusSeeOther)
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
