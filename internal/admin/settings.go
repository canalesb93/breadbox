package admin

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/db"
	plaidprovider "breadbox/internal/provider/plaid"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
)

// SettingsGetHandler serves GET /admin/settings.
func SettingsGetHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{
			"PageTitle":         "Settings",
			"CurrentPage":       "settings",
			"CSRFToken":         GetCSRFToken(r),
			"Flash":             GetFlash(r.Context(), sm),
			"SyncIntervalMinutes": a.Config.SyncIntervalMinutes,
			"WebhookURL":        a.Config.WebhookURL,
			"PlaidClientID":     a.Config.PlaidClientID,
			"PlaidSecret":       a.Config.PlaidSecret,
			"PlaidEnv":          a.Config.PlaidEnv,
			"PlaidFromEnv":      os.Getenv("PLAID_CLIENT_ID") != "",
		}
		tr.Render(w, r, "settings.html", data)
	}
}

// SettingsPostHandler serves POST /admin/settings.
func SettingsPostHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Parse form values.
		syncIntervalStr := r.FormValue("sync_interval_minutes")
		webhookURL := strings.TrimSpace(r.FormValue("webhook_url"))

		// Validate sync interval.
		syncInterval, err := strconv.Atoi(syncIntervalStr)
		if err != nil || !isValidSyncInterval(syncInterval) {
			SetFlash(ctx, sm, "error", "Invalid sync interval.")
			http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
			return
		}

		// Validate webhook URL.
		if webhookURL != "" && !strings.HasPrefix(webhookURL, "https://") {
			SetFlash(ctx, sm, "error", "Webhook URL must use HTTPS.")
			http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
			return
		}

		// Save sync settings.
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

		// Handle Plaid credentials if not set from environment.
		if os.Getenv("PLAID_CLIENT_ID") == "" {
			plaidClientID := strings.TrimSpace(r.FormValue("plaid_client_id"))
			plaidSecret := strings.TrimSpace(r.FormValue("plaid_secret"))
			plaidEnv := strings.TrimSpace(r.FormValue("plaid_env"))

			if plaidClientID != "" && plaidSecret != "" && plaidEnv != "" {
				// Validate credentials before saving.
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

		SetFlash(ctx, sm, "success", "Settings saved.")
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
