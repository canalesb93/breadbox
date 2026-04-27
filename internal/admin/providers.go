package admin

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/crypto"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	plaidprovider "breadbox/internal/provider/plaid"
	tellerprovider "breadbox/internal/provider/teller"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ProvidersGetHandler serves GET /admin/providers.
func ProvidersGetHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Fetch provider health summaries (connection counts, last sync info).
		providerHealth, err := svc.GetProviderHealthSummaries(ctx)
		if err != nil {
			a.Logger.Error("get provider health summaries", "error", err)
			providerHealth = map[string]*service.ProviderHealthSummary{}
		}

		// Ensure entries exist for all providers even if they have no connections.
		for _, p := range []string{"plaid", "teller", "csv"} {
			if _, ok := providerHealth[p]; !ok {
				providerHealth[p] = &service.ProviderHealthSummary{Provider: p}
			}
		}

		data := map[string]any{
			"PageTitle":   "Providers",
			"CurrentPage": "providers",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(r.Context(), sm),
		}

		props := pages.ProvidersProps{
			CSRFToken:     GetCSRFToken(r),
			ConfigSources: a.Config.ConfigSources,

			PlaidConfigured: a.Providers["plaid"] != nil,
			PlaidFromEnv:    os.Getenv("PLAID_CLIENT_ID") != "",
			PlaidClientID:   a.Config.PlaidClientID,
			PlaidEnv:        a.Config.PlaidEnv,
			WebhookURL:      a.Config.WebhookURL,

			TellerConfigured:        a.Providers["teller"] != nil,
			TellerFromEnv:           os.Getenv("TELLER_APP_ID") != "",
			TellerAppID:             a.Config.TellerAppID,
			TellerEnv:               a.Config.TellerEnv,
			TellerCertFromEnv:       a.Config.TellerCertPath != "",
			TellerCertConfigured:    a.Config.TellerCertPath != "" || len(a.Config.TellerCertPEM) > 0,
			TellerWebhookConfigured: a.Config.TellerWebhookSecret != "",

			HasEncryptionKey:    len(a.Config.EncryptionKey) > 0,
			SyncIntervalMinutes: a.Config.SyncIntervalMinutes,
			ProviderHealth:      providerHealth,
		}
		renderProviders(w, r, sm, tr, data, props)
	}
}

// renderProviders wraps the Providers tab body in the unified Settings shell.
func renderProviders(w http.ResponseWriter, r *http.Request, sm *scs.SessionManager, tr *TemplateRenderer, data map[string]any, props pages.ProvidersProps) {
	renderSettingsTab(tr, w, r, sm, data, pages.SettingsTabProviders, pages.Providers(props))
}

// ProvidersSavePlaidHandler serves POST /admin/providers/plaid.
func ProvidersSavePlaidHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if os.Getenv("PLAID_CLIENT_ID") != "" {
			FlashRedirect(w, r, sm, "error", "Plaid is configured via environment variables and cannot be changed here.", "/settings/providers")
			return
		}

		plaidClientID := strings.TrimSpace(r.FormValue("plaid_client_id"))
		plaidSecret := strings.TrimSpace(r.FormValue("plaid_secret"))
		plaidEnv := strings.TrimSpace(r.FormValue("plaid_env"))
		webhookURL := strings.TrimSpace(r.FormValue("webhook_url"))

		// If secret is empty, keep the existing one (user didn't change it).
		if plaidSecret == "" {
			plaidSecret = a.Config.PlaidSecret
		}

		if plaidClientID == "" {
			// Clearing Plaid config.
			for _, key := range []string{"plaid_client_id", "plaid_secret", "plaid_env", "webhook_url"} {
				_ = a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
					Key: key, Value: pgtype.Text{},
				})
			}
			a.Config.PlaidClientID = ""
			a.Config.PlaidSecret = ""
			a.Config.PlaidEnv = "sandbox"
			a.Config.WebhookURL = ""
			_ = a.ReinitProvider("plaid")
			FlashRedirect(w, r, sm, "success", "Plaid configuration cleared.", "/settings/providers")
			return
		}

		if plaidSecret == "" {
			FlashRedirect(w, r, sm, "error", "Plaid secret is required.", "/settings/providers")
			return
		}

		if webhookURL != "" && !strings.HasPrefix(webhookURL, "https://") {
			FlashRedirect(w, r, sm, "error", "Webhook URL must use HTTPS.", "/settings/providers")
			return
		}

		if err := plaidprovider.ValidateCredentials(ctx, plaidClientID, plaidSecret, plaidEnv); err != nil {
			FlashRedirect(w, r, sm, "error", "Invalid Plaid credentials: "+err.Error(), "/settings/providers")
			return
		}

		entries := []db.SetAppConfigParams{
			{Key: "plaid_client_id", Value: pgconv.Text(plaidClientID)},
			{Key: "plaid_secret", Value: pgconv.Text(plaidSecret)},
			{Key: "plaid_env", Value: pgconv.Text(plaidEnv)},
			{Key: "webhook_url", Value: pgconv.TextIfNotEmpty(webhookURL)},
		}
		for _, entry := range entries {
			if err := a.Queries.SetAppConfig(ctx, entry); err != nil {
				a.Logger.Error("save plaid config", "error", err, "key", entry.Key)
				FlashRedirect(w, r, sm, "error", "Failed to save Plaid credentials.", "/settings/providers")
				return
			}
		}

		a.Config.PlaidClientID = plaidClientID
		a.Config.PlaidSecret = plaidSecret
		a.Config.PlaidEnv = plaidEnv
		a.Config.WebhookURL = webhookURL
		a.Config.ConfigSources["plaid_client_id"] = "db"
		a.Config.ConfigSources["plaid_secret"] = "db"
		a.Config.ConfigSources["plaid_env"] = "db"
		a.Config.ConfigSources["webhook_url"] = "db"

		if err := a.ReinitProvider("plaid"); err != nil {
			a.Logger.Error("reinit plaid provider", "error", err)
			FlashRedirect(w, r, sm, "error", "Plaid credentials saved but provider failed to initialize: "+err.Error(), "/settings/providers")
			return
		}

		SetFlash(ctx, sm, "success", "Plaid configuration saved and provider initialized.")
		http.Redirect(w, r, "/settings/providers", http.StatusSeeOther)
	}
}

// ProvidersSaveTellerHandler serves POST /admin/providers/teller.
func ProvidersSaveTellerHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if os.Getenv("TELLER_APP_ID") != "" {
			FlashRedirect(w, r, sm, "error", "Teller is configured via environment variables and cannot be changed here.", "/settings/providers")
			return
		}

		// Parse multipart form (10MB max for cert/key files).
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to parse form data.", "/settings/providers")
			return
		}

		tellerAppID := strings.TrimSpace(r.FormValue("teller_app_id"))
		tellerEnv := strings.TrimSpace(r.FormValue("teller_env"))
		tellerWebhookSecret := strings.TrimSpace(r.FormValue("teller_webhook_secret"))

		if tellerAppID == "" {
			// Clearing Teller config.
			for _, key := range []string{"teller_app_id", "teller_env", "teller_webhook_secret", "teller_cert_pem", "teller_key_pem"} {
				_ = a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
					Key: key, Value: pgtype.Text{},
				})
			}
			a.Config.TellerAppID = ""
			a.Config.TellerEnv = "sandbox"
			a.Config.TellerWebhookSecret = ""
			a.Config.TellerCertPEM = nil
			a.Config.TellerKeyPEM = nil
			_ = a.ReinitProvider("teller")
			FlashRedirect(w, r, sm, "success", "Teller configuration cleared.", "/settings/providers")
			return
		}

		validTellerEnvs := map[string]bool{"sandbox": true, "development": true, "production": true}
		if !validTellerEnvs[tellerEnv] {
			tellerEnv = "sandbox"
		}

		// Save app ID, env, webhook secret.
		configEntries := []db.SetAppConfigParams{
			{Key: "teller_app_id", Value: pgconv.Text(tellerAppID)},
			{Key: "teller_env", Value: pgconv.Text(tellerEnv)},
		}
		if tellerWebhookSecret != "" {
			configEntries = append(configEntries, db.SetAppConfigParams{
				Key: "teller_webhook_secret", Value: pgconv.Text(tellerWebhookSecret),
			})
		}
		for _, entry := range configEntries {
			if err := a.Queries.SetAppConfig(ctx, entry); err != nil {
				a.Logger.Error("save teller config", "error", err, "key", entry.Key)
				FlashRedirect(w, r, sm, "error", "Failed to save Teller configuration.", "/settings/providers")
				return
			}
		}

		a.Config.TellerAppID = tellerAppID
		a.Config.TellerEnv = tellerEnv
		if tellerWebhookSecret != "" {
			a.Config.TellerWebhookSecret = tellerWebhookSecret
		}
		a.Config.ConfigSources["teller_app_id"] = "db"
		a.Config.ConfigSources["teller_env"] = "db"

		// Handle certificate file uploads (only if not configured from env).
		if a.Config.TellerCertPath == "" {
			certPEM, keyPEM, err := readTellerCertFiles(r)
			if err != nil {
				FlashRedirect(w, r, sm, "error", err.Error(), "/settings/providers")
				return
			}

			if certPEM != nil && keyPEM != nil {
				// Validate the key pair.
				if err := tellerprovider.ValidateCredentialsPEM(certPEM, keyPEM); err != nil {
					FlashRedirect(w, r, sm, "error", "Invalid certificate/key: "+err.Error(), "/settings/providers")
					return
				}

				if len(a.Config.EncryptionKey) == 0 {
					FlashRedirect(w, r, sm, "error", "Encryption key is required to store certificates. Set ENCRYPTION_KEY environment variable.", "/settings/providers")
					return
				}

				// Encrypt and store.
				encCert, err := crypto.Encrypt(certPEM, a.Config.EncryptionKey)
				if err != nil {
					FlashRedirect(w, r, sm, "error", "Failed to encrypt certificate.", "/settings/providers")
					return
				}
				encKey, err := crypto.Encrypt(keyPEM, a.Config.EncryptionKey)
				if err != nil {
					FlashRedirect(w, r, sm, "error", "Failed to encrypt private key.", "/settings/providers")
					return
				}

				certB64 := base64.StdEncoding.EncodeToString(encCert)
				keyB64 := base64.StdEncoding.EncodeToString(encKey)

				if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
					Key: "teller_cert_pem", Value: pgconv.Text(certB64),
				}); err != nil {
					FlashRedirect(w, r, sm, "error", "Failed to save certificate.", "/settings/providers")
					return
				}
				if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
					Key: "teller_key_pem", Value: pgconv.Text(keyB64),
				}); err != nil {
					FlashRedirect(w, r, sm, "error", "Failed to save private key.", "/settings/providers")
					return
				}

				a.Config.TellerCertPEM = certPEM
				a.Config.TellerKeyPEM = keyPEM
			}
		}

		if err := a.ReinitProvider("teller"); err != nil {
			a.Logger.Error("reinit teller provider", "error", err)
			FlashRedirect(w, r, sm, "error", "Teller settings saved but provider failed to initialize: "+err.Error(), "/settings/providers")
			return
		}

		SetFlash(ctx, sm, "success", "Teller configuration saved.")
		http.Redirect(w, r, "/settings/providers", http.StatusSeeOther)
	}
}

// readTellerCertFiles reads the uploaded certificate and key PEM files.
// Returns nil,nil,nil if no files were uploaded.
func readTellerCertFiles(r *http.Request) (certPEM, keyPEM []byte, err error) {
	certFile, _, certErr := r.FormFile("teller_cert_file")
	keyFile, _, keyErr := r.FormFile("teller_key_file")

	hasCert := certErr == nil
	hasKey := keyErr == nil

	if !hasCert && !hasKey {
		return nil, nil, nil
	}
	if hasCert != hasKey {
		if hasCert {
			certFile.Close()
		}
		if hasKey {
			keyFile.Close()
		}
		return nil, nil, fmt.Errorf("both certificate and private key files must be uploaded together")
	}

	defer certFile.Close()
	defer keyFile.Close()

	certPEM, err = io.ReadAll(io.LimitReader(certFile, 64*1024))
	if err != nil {
		return nil, nil, fmt.Errorf("read certificate file: %w", err)
	}
	keyPEM, err = io.ReadAll(io.LimitReader(keyFile, 64*1024))
	if err != nil {
		return nil, nil, fmt.Errorf("read private key file: %w", err)
	}

	return certPEM, keyPEM, nil
}

// ProvidersTestHandler serves POST /admin/api/test-provider/{provider}.
func ProvidersTestHandler(a *app.App) http.HandlerFunc {
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
			// Test with file paths first, then PEM bytes.
			if a.Config.TellerCertPath != "" && a.Config.TellerKeyPath != "" {
				err := tellerprovider.ValidateCredentials(a.Config.TellerCertPath, a.Config.TellerKeyPath)
				if err != nil {
					writeJSON(w, http.StatusOK, testResult{Provider: "teller", Success: false, Message: err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, testResult{Provider: "teller", Success: true, Message: "Certificate valid"})
				return
			}
			if len(a.Config.TellerCertPEM) > 0 && len(a.Config.TellerKeyPEM) > 0 {
				err := tellerprovider.ValidateCredentialsPEM(a.Config.TellerCertPEM, a.Config.TellerKeyPEM)
				if err != nil {
					writeJSON(w, http.StatusOK, testResult{Provider: "teller", Success: false, Message: err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, testResult{Provider: "teller", Success: true, Message: "Certificate valid"})
				return
			}
			writeJSON(w, http.StatusOK, testResult{Provider: "teller", Success: false, Message: "Teller certificate not configured"})

		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Unknown provider: " + providerName})
		}
	}
}
