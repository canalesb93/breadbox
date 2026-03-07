package admin

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/db"
	plaidprovider "breadbox/internal/provider/plaid"
	tellerprovider "breadbox/internal/provider/teller"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
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
		var pgVersion string
		_ = a.DB.QueryRow(ctx, "SELECT version()").Scan(&pgVersion)

		data := map[string]any{
			"PageTitle":         "Settings",
			"CurrentPage":       "settings",
			"CSRFToken":         GetCSRFToken(r),
			"Flash":             GetFlash(ctx, sm),
			"SyncIntervalMinutes": a.Config.SyncIntervalMinutes,
			"WebhookURL":        a.Config.WebhookURL,
			"PlaidClientID":     a.Config.PlaidClientID,
			"PlaidSecret":       a.Config.PlaidSecret,
			"PlaidEnv":          a.Config.PlaidEnv,
			"PlaidFromEnv":      os.Getenv("PLAID_CLIENT_ID") != "",
			"TellerAppID":            a.Config.TellerAppID,
			"TellerEnv":              a.Config.TellerEnv,
			"TellerFromEnv":          os.Getenv("TELLER_APP_ID") != "",
			"TellerCertConfigured":   a.Config.TellerCertPath != "" && a.Config.TellerKeyPath != "",
			"TellerWebhookConfigured": a.Config.TellerWebhookSecret != "",
			// System info (13B.4)
			"Version":         a.Config.Version,
			"GoVersion":       runtime.Version(),
			"PostgresVersion": pgVersion,
			"Uptime":          formatUptime(time.Since(a.Config.StartTime)),
			"ProviderCount":   len(a.Providers),
			// Config sources (13B.5)
			"ConfigSources":   a.Config.ConfigSources,
			// Safety indicators (13B.7)
			"HasEncryptionKey": len(a.Config.EncryptionKey) > 0,
			// Next sync time (17A.3)
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
		webhookURL := strings.TrimSpace(r.FormValue("webhook_url"))

		syncInterval, err := strconv.Atoi(syncIntervalStr)
		if err != nil || !isValidSyncInterval(syncInterval) {
			SetFlash(ctx, sm, "error", "Invalid sync interval.")
			http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
			return
		}

		if webhookURL != "" && !strings.HasPrefix(webhookURL, "https://") {
			SetFlash(ctx, sm, "error", "Webhook URL must use HTTPS.")
			http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
			return
		}

		if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "sync_interval_minutes",
			Value: pgtype.Text{String: fmt.Sprintf("%d", syncInterval), Valid: true},
		}); err != nil {
			a.Logger.Error("save sync interval", "error", err)
			SetFlash(ctx, sm, "error", "Failed to save sync interval.")
			http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
			return
		}
		a.Config.SyncIntervalMinutes = syncInterval

		if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "webhook_url",
			Value: pgtype.Text{String: webhookURL, Valid: webhookURL != ""},
		}); err != nil {
			a.Logger.Error("save webhook url", "error", err)
			SetFlash(ctx, sm, "error", "Failed to save webhook URL.")
			http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
			return
		}
		a.Config.WebhookURL = webhookURL

		SetFlash(ctx, sm, "success", "Sync settings saved.")
		http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
	}
}

// SettingsProvidersPostHandler serves POST /admin/settings/providers.
func SettingsProvidersPostHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Handle Plaid credentials if not set from environment.
		if os.Getenv("PLAID_CLIENT_ID") == "" {
			plaidClientID := strings.TrimSpace(r.FormValue("plaid_client_id"))
			plaidSecret := strings.TrimSpace(r.FormValue("plaid_secret"))
			plaidEnv := strings.TrimSpace(r.FormValue("plaid_env"))

			if plaidClientID != "" && plaidSecret != "" && plaidEnv != "" {
				if err := plaidprovider.ValidateCredentials(ctx, plaidClientID, plaidSecret, plaidEnv); err != nil {
					SetFlash(ctx, sm, "error", "Invalid Plaid credentials: "+err.Error())
					http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
					return
				}

				configEntries := []db.SetAppConfigParams{
					{Key: "plaid_client_id", Value: pgtype.Text{String: plaidClientID, Valid: true}},
					{Key: "plaid_secret", Value: pgtype.Text{String: plaidSecret, Valid: true}},
					{Key: "plaid_env", Value: pgtype.Text{String: plaidEnv, Valid: true}},
				}
				for _, entry := range configEntries {
					if err := a.Queries.SetAppConfig(ctx, entry); err != nil {
						a.Logger.Error("save plaid config", "error", err, "key", entry.Key)
						SetFlash(ctx, sm, "error", "Failed to save Plaid credentials.")
						http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
						return
					}
				}
				a.Config.PlaidClientID = plaidClientID
				a.Config.PlaidSecret = plaidSecret
				a.Config.PlaidEnv = plaidEnv
			}
		}

		// Handle Teller credentials if not set from environment.
		if os.Getenv("TELLER_APP_ID") == "" {
			tellerAppID := strings.TrimSpace(r.FormValue("teller_app_id"))
			tellerEnv := strings.TrimSpace(r.FormValue("teller_env"))

			if tellerAppID != "" {
				if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
					Key:   "teller_app_id",
					Value: pgtype.Text{String: tellerAppID, Valid: true},
				}); err != nil {
					a.Logger.Error("save teller app id", "error", err)
				} else {
					a.Config.TellerAppID = tellerAppID
				}
			}
			if tellerEnv != "" {
				validTellerEnvs := map[string]bool{"sandbox": true, "production": true}
				if validTellerEnvs[tellerEnv] {
					if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
						Key:   "teller_env",
						Value: pgtype.Text{String: tellerEnv, Valid: true},
					}); err != nil {
						a.Logger.Error("save teller env", "error", err)
					} else {
						a.Config.TellerEnv = tellerEnv
					}
				}
			}
		}

		SetFlash(ctx, sm, "success", "Provider settings saved.")
		http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
	}
}

// TestProviderHandler serves POST /admin/api/test-provider/{provider}.
func TestProviderHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providerName := chi.URLParam(r, "provider")

		type testResult struct {
			Provider string `json:"provider"`
			Success  bool   `json:"success"`
			Message  string `json:"message"`
		}

		switch providerName {
		case "plaid":
			if a.Config.PlaidClientID == "" || a.Config.PlaidSecret == "" {
				writeJSON(w, http.StatusOK, testResult{Provider: "plaid", Success: false, Message: "Plaid credentials not configured"})
				return
			}
			err := plaidprovider.ValidateCredentials(r.Context(), a.Config.PlaidClientID, a.Config.PlaidSecret, a.Config.PlaidEnv)
			if err != nil {
				writeJSON(w, http.StatusOK, testResult{Provider: "plaid", Success: false, Message: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, testResult{Provider: "plaid", Success: true, Message: "Connection successful"})

		case "teller":
			if a.Config.TellerCertPath == "" || a.Config.TellerKeyPath == "" {
				writeJSON(w, http.StatusOK, testResult{Provider: "teller", Success: false, Message: "Teller certificate not configured"})
				return
			}
			err := tellerprovider.ValidateCredentials(a.Config.TellerCertPath, a.Config.TellerKeyPath)
			if err != nil {
				writeJSON(w, http.StatusOK, testResult{Provider: "teller", Success: false, Message: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, testResult{Provider: "teller", Success: true, Message: "Certificate valid"})

		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Unknown provider: " + providerName})
		}
	}
}

// ChangePasswordHandler serves POST /admin/settings/password.
func ChangePasswordHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		adminIDStr := sm.GetString(ctx, sessionKeyAdminID)
		if adminIDStr == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		var adminID pgtype.UUID
		if err := adminID.Scan(adminIDStr); err != nil {
			SetFlash(ctx, sm, "error", "Invalid session.")
			http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
			return
		}

		admin, err := a.Queries.GetAdminAccountByID(ctx, adminID)
		if err != nil {
			SetFlash(ctx, sm, "error", "Account not found.")
			http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
			return
		}

		currentPassword := r.FormValue("current_password")
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_password")

		if err := bcrypt.CompareHashAndPassword(admin.HashedPassword, []byte(currentPassword)); err != nil {
			SetFlash(ctx, sm, "error", "Current password is incorrect.")
			http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
			return
		}

		if len(newPassword) < 8 {
			SetFlash(ctx, sm, "error", "New password must be at least 8 characters.")
			http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
			return
		}

		if newPassword != confirmPassword {
			SetFlash(ctx, sm, "error", "New passwords do not match.")
			http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
		if err != nil {
			SetFlash(ctx, sm, "error", "Failed to hash password.")
			http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
			return
		}

		if err := a.Queries.UpdateAdminPassword(ctx, db.UpdateAdminPasswordParams{
			ID:             adminID,
			HashedPassword: hashedPassword,
		}); err != nil {
			a.Logger.Error("update admin password", "error", err)
			SetFlash(ctx, sm, "error", "Failed to update password.")
			http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
			return
		}

		SetFlash(ctx, sm, "success", "Password updated successfully.")
		http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
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

