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

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
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

		data := map[string]any{
			"PageTitle":           "Settings",
			"CurrentPage":         "settings",
			"CSRFToken":           GetCSRFToken(r),
			"Flash":               GetFlash(ctx, sm),
			"SyncIntervalMinutes": a.Config.SyncIntervalMinutes,
			// Sync log retention
			"SyncLogRetentionDays": retentionDays,
			"SyncLogCount":         syncLogCount,
			// System info
			"Version":         a.Config.Version,
			"GoVersion":       runtime.Version(),
			"PostgresVersion": pgVersion,
			"Uptime":          formatUptime(time.Since(a.Config.StartTime)),
			"ProviderCount":   len(a.Providers),
			// Config sources
			"ConfigSources":   a.Config.ConfigSources,
			// Safety indicators
			"HasEncryptionKey": len(a.Config.EncryptionKey) > 0,
			// Next sync time
			"NextSyncTime": func() string {
				if a.Scheduler != nil {
					return formatNextSync(a.Scheduler.NextRun())
				}
				return ""
			}(),
		}
		tr.Render(w, r, "settings.html", data)
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
// Works for both admin_account and member_account sessions.
func ChangePasswordHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Check if this is a member account session.
		accountType := sm.GetString(ctx, sessionKeyAccountType)
		if accountType == "member_account" {
			memberIDStr := sm.GetString(ctx, sessionKeyMemberID)
			changePasswordFromMember(a, sm, w, r, memberIDStr, "/settings")
			return
		}

		// Legacy admin account flow.
		adminIDStr := sm.GetString(ctx, sessionKeyAdminID)
		if adminIDStr == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		var adminID pgtype.UUID
		if err := adminID.Scan(adminIDStr); err != nil {
			SetFlash(ctx, sm, "error", "Invalid session.")
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}

		admin, err := a.Queries.GetAdminAccountByID(ctx, adminID)
		if err != nil {
			SetFlash(ctx, sm, "error", "Account not found.")
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}

		currentPassword := r.FormValue("current_password")
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_password")

		if err := bcrypt.CompareHashAndPassword(admin.HashedPassword, []byte(currentPassword)); err != nil {
			SetFlash(ctx, sm, "error", "Current password is incorrect.")
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}

		if len(newPassword) < 8 {
			SetFlash(ctx, sm, "error", "New password must be at least 8 characters.")
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}

		if newPassword != confirmPassword {
			SetFlash(ctx, sm, "error", "New passwords do not match.")
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
		if err != nil {
			SetFlash(ctx, sm, "error", "Failed to hash password.")
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}

		if err := a.Queries.UpdateAdminPassword(ctx, db.UpdateAdminPasswordParams{
			ID:             adminID,
			HashedPassword: hashedPassword,
		}); err != nil {
			a.Logger.Error("update admin password", "error", err)
			SetFlash(ctx, sm, "error", "Failed to update password.")
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}

		SetFlash(ctx, sm, "success", "Password updated successfully.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
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

