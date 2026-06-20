//go:build !headless && !lite

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
	"breadbox/internal/avatar"
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

// serverZoneLabel describes the server process's local zone for the
// "Server default" timezone option, so a self-hoster who hasn't set an
// instance zone can still see what cron evaluates in. Format: the zone
// abbreviation plus its current UTC offset (e.g. "PDT · UTC-07:00"), or just
// the offset when the runtime only knows a numeric zone (e.g. "UTC+05:30").
// Read at the current instant, so it reflects the active DST offset.
func serverZoneLabel() string {
	name, offsetSec := time.Now().Zone()
	sign := "+"
	if offsetSec < 0 {
		sign = "-"
		offsetSec = -offsetSec
	}
	off := fmt.Sprintf("UTC%s%02d:%02d", sign, offsetSec/3600, (offsetSec%3600)/60)
	name = strings.TrimSpace(name)
	// A nameless zone surfaces as a numeric abbreviation like "+0530"; don't
	// pair that with the offset (redundant). Show the offset alone.
	if name == "" || strings.HasPrefix(name, "+") || strings.HasPrefix(name, "-") {
		return off
	}
	return name + " · " + off
}

// buildSettingsProps assembles the typed SettingsProps shared by every
// /settings/* General-family tab (General, System, Help). Each
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
	// User style: prefer the new per-actor key; fall back to the
	// legacy single-key for deployments that haven't migrated yet so
	// the System tab still shows the operator's prior choice.
	userAvatarStyle := appconfig.String(ctx, a.Queries, appconfig.KeyAvatarUserStyle, "")
	if userAvatarStyle == "" {
		userAvatarStyle = appconfig.String(ctx, a.Queries, appconfig.KeyAvatarStyle, avatar.DefaultUserStyle)
	}
	agentAvatarStyle := appconfig.String(ctx, a.Queries, appconfig.KeyAvatarAgentStyle, avatar.DefaultAgentStyle)

	// Counterparty logo hotlinking (env → app_config → default) for the
	// Appearance section's toggle + optional logo.dev publishable token.
	counterpartyLogos, logoDevToken := a.Service.CounterpartyLogoSettings(ctx)

	nextSyncTime := ""
	if a.Scheduler != nil {
		nextSyncTime = formatNextSync(a.Scheduler.NextRun())
	}

	syncSchedules, _ := a.Service.ListSyncSchedules(ctx)
	newScheduleForm, editScheduleForms := buildScheduleDrawerForms(a, r, syncSchedules)

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
		SyncSchedules:        syncSchedules,
		NewScheduleForm:      newScheduleForm,
		EditScheduleForms:    editScheduleForms,
		InstanceTimezone:     appconfig.String(ctx, a.Queries, appconfig.KeyInstanceTimezone, ""),
		ServerZoneLabel:      serverZoneLabel(),
		ConfigSources:        a.Config.ConfigSources,
		AvatarUserStyle:      userAvatarStyle,
		AvatarAgentStyle:     agentAvatarStyle,
		AvatarStyles:         avatar.AvailableStyles,
		CounterpartyLogos:    counterpartyLogos,
		LogoDevToken:         logoDevToken,
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

// SettingsGetHandler serves GET /settings — the General tab is the
// default landing for the Settings entry. /settings/general renders
// the same.
func SettingsGetHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		props, data := buildSettingsProps(a, r)
		data["PageTitle"] = "General"
		data["CurrentPage"] = "general"
		data["Flash"] = GetFlash(r.Context(), sm)
		renderSettingsTab(tr, w, r, data, pages.SettingsTabGeneral, pages.SettingsGeneral(props))
	}
}

// SystemSettingsHandler serves GET /settings/system — combines security
// (encryption key) and runtime/version info on a single tab.
func SystemSettingsHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		props, data := buildSettingsProps(a, r)
		data["PageTitle"] = "System"
		data["CurrentPage"] = "system"
		data["Flash"] = GetFlash(r.Context(), sm)
		renderSettingsTab(tr, w, r, data, pages.SettingsTabSystem, pages.SettingsSystem(props))
	}
}

// HelpSettingsHandler serves GET /settings/help.
func HelpSettingsHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		props, data := buildSettingsProps(a, r)
		data["PageTitle"] = "Help"
		data["CurrentPage"] = "help"
		data["Flash"] = GetFlash(r.Context(), sm)
		renderSettingsTab(tr, w, r, data, pages.SettingsTabHelp, pages.SettingsHelp(props))
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

// SettingsTimezonePostHandler serves POST /settings/timezone — sets the
// instance IANA timezone all cron schedules (sync + workflows) are evaluated
// in. Empty clears it (back to the server's local zone). Auto-save: 204 on ok.
func SettingsTimezonePostHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tz := strings.TrimSpace(r.FormValue("instance_timezone"))
		if tz != "" {
			if _, err := time.LoadLocation(tz); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		if err := a.Queries.SetAppConfig(r.Context(), db.SetAppConfigParams{
			Key:   appconfig.KeyInstanceTimezone,
			Value: pgconv.Text(tz),
		}); err != nil {
			a.Logger.Error("save instance timezone", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// SettingsAvatarStylePostHandler serves POST /settings/avatar-style.
// Routes by the `actor` form field: "user" (default) persists into
// KeyAvatarUserStyle, "agent" into KeyAvatarAgentStyle. Updates the
// in-process default so the next /avatars/{id} render uses it.
// Uploaded avatars are untouched.
func SettingsAvatarStylePostHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		style := strings.TrimSpace(r.FormValue("avatar_style"))
		if !avatar.IsValidStyle(style) {
			FlashRedirect(w, r, sm, "error", "Invalid avatar style.", "/settings/general")
			return
		}

		actor := strings.TrimSpace(r.FormValue("actor"))
		if actor == "" {
			actor = "user"
		}
		var (
			key       string
			applyFunc func(string)
			label     string
		)
		switch actor {
		case "agent":
			key = appconfig.KeyAvatarAgentStyle
			applyFunc = avatar.SetAgentStyle
			label = "Agent avatar style"
		case "user":
			key = appconfig.KeyAvatarUserStyle
			applyFunc = avatar.SetUserStyle
			label = "User avatar style"
		default:
			FlashRedirect(w, r, sm, "error", "Invalid actor type.", "/settings/system")
			return
		}

		if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   key,
			Value: pgconv.Text(style),
		}); err != nil {
			a.Logger.Error("save avatar style", "error", err, "actor", actor)
			FlashRedirect(w, r, sm, "error", "Failed to save avatar style.", "/settings/general")
			return
		}
		applyFunc(style)

		SetFlash(ctx, sm, "success", label+" updated.")
		http.Redirect(w, r, "/settings/general", http.StatusSeeOther)
	}
}

// SettingsCounterpartyLogosPostHandler serves POST /settings/counterparty-logos
// — the auto-save toggle for hotlinking brand logos from logo.dev onto
// counterparty avatars. An unchecked checkbox omits the field, so absence = off.
// Returns 204 (the auto-save form shows its own toast).
func SettingsCounterpartyLogosPostHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enabled := r.FormValue("counterparty_logos") == "true"
		val := "false"
		if enabled {
			val = "true"
		}
		if err := a.Queries.SetAppConfig(r.Context(), db.SetAppConfigParams{
			Key:   appconfig.KeyCounterpartyLogos,
			Value: pgconv.Text(val),
		}); err != nil {
			a.Logger.Error("save counterparty logos toggle", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// SettingsLogoDevTokenPostHandler serves POST /settings/logo-dev-token — the
// auto-save input for the optional logo.dev publishable key appended to
// hotlinked logo URLs. The key is public by design (it rides in the <img src>),
// so it's stored in plaintext. Empty clears it. Returns 204.
func SettingsLogoDevTokenPostHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(r.FormValue("logo_dev_token"))
		if err := a.Queries.SetAppConfig(r.Context(), db.SetAppConfigParams{
			Key:   appconfig.KeyLogoDevToken,
			Value: pgconv.Text(token),
		}); err != nil {
			a.Logger.Error("save logo.dev token", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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
