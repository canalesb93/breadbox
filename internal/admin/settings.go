package admin

import (
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/db"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
)

// formatNextSync returns a human-readable string for the next sync time.
func formatNextSync(nextRun time.Time) string {
	if nextRun.IsZero() {
		return ""
	}
	d := time.Until(nextRun)
	if d <= 0 {
		return "any moment now"
	}
	return "in " + formatUptime(d)
}

// SettingsGetHandler serves GET /admin/settings.
func SettingsGetHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// System info.
		var pgVersionRaw string
		_ = a.DB.QueryRow(ctx, "SELECT version()").Scan(&pgVersionRaw)
		pgVersionRaw = strings.TrimPrefix(pgVersionRaw, "PostgreSQL ")
		// Extract just the version number (e.g., "16.13") from the full string.
		pgVersion := pgVersionRaw
		if idx := strings.IndexByte(pgVersionRaw, ' '); idx > 0 {
			pgVersion = pgVersionRaw[:idx]
		}

		// Sync log retention.
		retentionDays, _ := a.Service.GetSyncLogRetentionDays(ctx)
		syncLogCount, _ := a.Service.CountSyncLogs(ctx)

		// Check onboarding dismissed state for help section.
		onboardingDismissed := GetConfigBool(ctx, a.Queries, "onboarding_dismissed")

		nextSyncTime := ""
		if a.Scheduler != nil {
			nextSyncTime = formatNextSync(a.Scheduler.NextRun())
		}
		data := map[string]any{
			"PageTitle":   "Settings",
			"CurrentPage": "settings",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
		}
		body := pages.Settings(pages.SettingsProps{
			CSRFToken:            GetCSRFToken(r),
			SyncIntervalMinutes:  a.Config.SyncIntervalMinutes,
			SyncLogRetentionDays: retentionDays,
			SyncLogCount:         syncLogCount,
			Version:              a.Config.Version,
			GoVersion:            runtime.Version(),
			PostgresVersion:      pgVersion,
			Uptime:               formatUptime(time.Since(a.Config.StartTime)),
			ProviderCount:        len(a.Providers),
			HasEncryptionKey:     len(a.Config.EncryptionKey) > 0,
			OnboardingDismissed:  onboardingDismissed,
			NextSyncTime:         nextSyncTime,
			ConfigSources:        a.Config.ConfigSources,
		})
		tr.RenderWithTempl(w, r, data, body)
	}
}

// SettingsSyncPostHandler serves POST /admin/settings/sync.
func SettingsSyncPostHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		syncIntervalStr := r.FormValue("sync_interval_minutes")

		syncInterval, err := strconv.Atoi(syncIntervalStr)
		if err != nil || !isValidSyncInterval(syncInterval) {
			SetFlash(ctx, sm, "error", "Invalid sync interval.")
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}

		if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "sync_interval_minutes",
			Value: pgtype.Text{String: fmt.Sprintf("%d", syncInterval), Valid: true},
		}); err != nil {
			a.Logger.Error("save sync interval", "error", err)
			SetFlash(ctx, sm, "error", "Failed to save sync interval.")
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}
		a.Config.SyncIntervalMinutes = syncInterval

		SetFlash(ctx, sm, "success", "Sync settings saved.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
	}
}

// ChangePasswordHandler serves POST /admin/settings/password.
// Works for all account types via the unified auth_accounts table.
func ChangePasswordHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountIDStr := SessionAccountID(sm, r)
		if accountIDStr == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		changePasswordForAccount(a, sm, w, r, accountIDStr, "/settings")
	}
}

// SettingsRetentionPostHandler serves POST /admin/settings/retention.
func SettingsRetentionPostHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		retentionStr := r.FormValue("sync_log_retention_days")
		retentionDays, err := strconv.Atoi(retentionStr)
		if err != nil || retentionDays < 0 || retentionDays > 3650 {
			SetFlash(ctx, sm, "error", "Invalid retention period. Must be 0-3650 days.")
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}

		if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "sync_log_retention_days",
			Value: pgtype.Text{String: fmt.Sprintf("%d", retentionDays), Valid: true},
		}); err != nil {
			a.Logger.Error("save sync log retention", "error", err)
			SetFlash(ctx, sm, "error", "Failed to save retention setting.")
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}

		if retentionDays == 0 {
			SetFlash(ctx, sm, "success", "Sync log cleanup disabled.")
		} else {
			SetFlash(ctx, sm, "success", fmt.Sprintf("Sync log retention set to %d days.", retentionDays))
		}
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
	}
}

func isValidSyncInterval(minutes int) bool {
	valid := map[int]bool{
		15: true, 30: true, 60: true, // sub-hour
		240: true, 480: true, 720: true, 1440: true, // 4h, 8h, 12h, 24h
	}
	return valid[minutes]
}

// formatUptime formats a duration into a human-readable string.
func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

