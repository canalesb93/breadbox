package admin

import (
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/appconfig"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
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

// buildSettingsProps assembles the typed SettingsProps shared by every
// /settings/* General-family tab (Sync, Security, System, Help). Each
// tab handler builds full props (cheap — a few DB lookups + version
// check) and renders only the section it owns.
func buildSettingsProps(a *app.App, r *http.Request) (pages.SettingsProps, map[string]any) {
	ctx := r.Context()

	// System info.
	var pgVersionRaw string
	_ = a.DB.QueryRow(ctx, "SELECT version()").Scan(&pgVersionRaw)
	pgVersionRaw = strings.TrimPrefix(pgVersionRaw, "PostgreSQL ")
	pgVersion := pgVersionRaw
	if idx := strings.IndexByte(pgVersionRaw, ' '); idx > 0 {
		pgVersion = pgVersionRaw[:idx]
	}

	retentionDays, _ := a.Service.GetSyncLogRetentionDays(ctx)
	syncLogCount, _ := a.Service.CountSyncLogs(ctx)
	onboardingDismissed := appconfig.Bool(ctx, a.Queries, "onboarding_dismissed", false)

	nextSyncTime := ""
	if a.Scheduler != nil {
		nextSyncTime = formatNextSync(a.Scheduler.NextRun())
	}

	props := pages.SettingsProps{
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
	}
	if a.VersionChecker != nil {
		if updateAvailable, latest, err := a.VersionChecker.CheckForUpdate(ctx); err == nil && updateAvailable != nil && *updateAvailable && latest != nil {
			props.UpdateAvailable = true
			props.LatestVersion = latest.Version
			props.LatestURL = latest.URL
		}
	}

	data := map[string]any{
		"CSRFToken": GetCSRFToken(r),
		"Flash":     nil,
	}
	return props, data
}

// SettingsGetHandler serves GET /settings — the Sync tab is the default
// landing for the Settings entry. /settings/sync renders the same.
func SettingsGetHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		props, data := buildSettingsProps(a, r)
		data["PageTitle"] = "Sync"
		data["CurrentPage"] = "sync"
		data["Flash"] = GetFlash(r.Context(), sm)
		renderSettingsTab(tr, w, r, sm, data, pages.SettingsTabSync, pages.SettingsSync(props))
	}
}

// SecuritySettingsHandler serves GET /settings/security.
func SecuritySettingsHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		props, data := buildSettingsProps(a, r)
		data["PageTitle"] = "Security"
		data["CurrentPage"] = "security"
		data["Flash"] = GetFlash(r.Context(), sm)
		renderSettingsTab(tr, w, r, sm, data, pages.SettingsTabSecurity, pages.SettingsSecurity(props))
	}
}

// SystemSettingsHandler serves GET /settings/system.
func SystemSettingsHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		props, data := buildSettingsProps(a, r)
		data["PageTitle"] = "System"
		data["CurrentPage"] = "system"
		data["Flash"] = GetFlash(r.Context(), sm)
		renderSettingsTab(tr, w, r, sm, data, pages.SettingsTabSystem, pages.SettingsSystem(props))
	}
}

// HelpSettingsHandler serves GET /settings/help.
func HelpSettingsHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		props, data := buildSettingsProps(a, r)
		data["PageTitle"] = "Help"
		data["CurrentPage"] = "help"
		data["Flash"] = GetFlash(r.Context(), sm)
		renderSettingsTab(tr, w, r, sm, data, pages.SettingsTabHelp, pages.SettingsHelp(props))
	}
}

// SettingsSyncPostHandler serves POST /admin/settings/sync.
func SettingsSyncPostHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		syncIntervalStr := r.FormValue("sync_interval_minutes")

		syncInterval, err := strconv.Atoi(syncIntervalStr)
		if err != nil || !isValidSyncInterval(syncInterval) {
			FlashRedirect(w, r, sm, "error", "Invalid sync interval.", "/settings")
			return
		}

		if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "sync_interval_minutes",
			Value: pgconv.Text(strconv.Itoa(syncInterval)),
		}); err != nil {
			a.Logger.Error("save sync interval", "error", err)
			FlashRedirect(w, r, sm, "error", "Failed to save sync interval.", "/settings")
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
			FlashRedirect(w, r, sm, "error", "Invalid retention period. Must be 0-3650 days.", "/settings")
			return
		}

		if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "sync_log_retention_days",
			Value: pgconv.Text(strconv.Itoa(retentionDays)),
		}); err != nil {
			a.Logger.Error("save sync log retention", "error", err)
			FlashRedirect(w, r, sm, "error", "Failed to save retention setting.", "/settings")
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

