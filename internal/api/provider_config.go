//go:build !lite

package api

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/crypto"
	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	tellerprovider "breadbox/internal/provider/teller"
)

// Provider configuration REST endpoints. These wrap the same storage path as
// the admin form handlers (internal/admin/providers.go) so that headless
// installs can bootstrap providers without an admin UI session.
//
// Sensitive fields (Plaid secret, Teller PEM cert/key) are AES-256-GCM
// encrypted at rest using the same crypto helpers as the admin form, and
// are redacted on GET — callers can only see boolean "*_set" flags, never
// raw values. Empty values on PUT preserve the existing stored value, which
// matches the admin form's "leave blank to keep" UX for not retyping
// secrets.
//
// All three handlers take *app.App rather than *service.Service because:
//   - the encryption key lives on app.Config (not service)
//   - app.ReinitProvider hot-reloads the live provider map after a write
//   - app.Config in-memory state must stay in sync with app_config DB rows
//
// Mirroring this in the service layer would require lifting EncryptionKey,
// the live config struct, and the provider map out of *app.App, which is a
// much larger refactor than this bundle warrants.

// providerConfigResponse is the wire shape of GET /settings/providers.
type providerConfigResponse struct {
	Plaid  plaidConfigView  `json:"plaid"`
	Teller tellerConfigView `json:"teller"`
}

type plaidConfigView struct {
	Configured  bool   `json:"configured"`
	FromEnv     bool   `json:"from_env"`
	ClientID    string `json:"client_id,omitempty"`
	Environment string `json:"environment,omitempty"`
	WebhookURL  string `json:"webhook_url,omitempty"`
	SecretSet   bool   `json:"secret_set"`
}

type tellerConfigView struct {
	Configured       bool   `json:"configured"`
	FromEnv          bool   `json:"from_env"`
	ApplicationID    string `json:"application_id,omitempty"`
	Environment      string `json:"environment,omitempty"`
	CertificateSet   bool   `json:"certificate_set"`
	WebhookSecretSet bool   `json:"webhook_secret_set"`
}

type updatePlaidConfigRequest struct {
	ClientID    string  `json:"client_id"`
	Secret      *string `json:"secret"`
	Environment string  `json:"environment"`
	WebhookURL  string  `json:"webhook_url"`
}

type updateTellerConfigRequest struct {
	ApplicationID    string  `json:"application_id"`
	Environment      string  `json:"environment"`
	Certificate      *string `json:"certificate"`
	PrivateKey       *string `json:"private_key"`
	WebhookSecret    *string `json:"webhook_secret"`
}

var (
	validPlaidEnvs  = map[string]bool{"sandbox": true, "development": true, "production": true}
	validTellerEnvs = map[string]bool{"sandbox": true, "development": true, "production": true}
)

// GetProviderConfigHandler serves GET /api/v1/settings/providers.
func GetProviderConfigHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeData(w, buildProviderConfigResponse(a))
	}
}

// UpdatePlaidConfigHandler serves PUT /api/v1/settings/providers/plaid.
func UpdatePlaidConfigHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("PLAID_CLIENT_ID") != "" {
			mw.WriteError(w, http.StatusConflict, "PROVIDER_FROM_ENV",
				"Plaid is configured via environment variables and cannot be changed via the API")
			return
		}

		var req updatePlaidConfigRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		clientID := strings.TrimSpace(req.ClientID)
		if clientID == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"client_id is required")
			return
		}

		env := strings.TrimSpace(req.Environment)
		if env == "" {
			env = "sandbox"
		}
		if !validPlaidEnvs[env] {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"environment must be one of: sandbox, development, production")
			return
		}

		webhookURL := strings.TrimSpace(req.WebhookURL)
		if webhookURL != "" && !strings.HasPrefix(webhookURL, "https://") {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"webhook_url must use https://")
			return
		}

		// Empty / nil secret means "keep existing" — matches the admin form
		// UX where users don't have to retype the secret on every save.
		secret := a.Config.PlaidSecret
		if req.Secret != nil {
			trimmed := strings.TrimSpace(*req.Secret)
			if trimmed != "" {
				secret = trimmed
			}
		}
		if secret == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"secret is required when no existing secret is stored")
			return
		}

		entries := []db.SetAppConfigParams{
			{Key: "plaid_client_id", Value: pgconv.Text(clientID)},
			{Key: "plaid_secret", Value: pgconv.Text(secret)},
			{Key: "plaid_env", Value: pgconv.Text(env)},
			{Key: "webhook_url", Value: pgconv.TextIfNotEmpty(webhookURL)},
		}
		for _, entry := range entries {
			if err := a.Queries.SetAppConfig(r.Context(), entry); err != nil {
				a.Logger.Error("save plaid config", "error", err, "key", entry.Key)
				mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
					"Failed to save Plaid configuration")
				return
			}
		}

		a.Config.PlaidClientID = clientID
		a.Config.PlaidSecret = secret
		a.Config.PlaidEnv = env
		a.Config.WebhookURL = webhookURL
		a.Config.ConfigSources["plaid_client_id"] = "db"
		a.Config.ConfigSources["plaid_secret"] = "db"
		a.Config.ConfigSources["plaid_env"] = "db"
		a.Config.ConfigSources["webhook_url"] = "db"

		if err := a.ReinitProvider("plaid"); err != nil {
			a.Logger.Error("reinit plaid provider", "error", err)
			mw.WriteError(w, http.StatusInternalServerError, "PROVIDER_REINIT_FAILED",
				"Plaid configuration saved but provider failed to initialize: "+err.Error())
			return
		}

		writeData(w, buildProviderConfigResponse(a))
	}
}

// UpdateTellerConfigHandler serves PUT /api/v1/settings/providers/teller.
func UpdateTellerConfigHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("TELLER_APP_ID") != "" {
			mw.WriteError(w, http.StatusConflict, "PROVIDER_FROM_ENV",
				"Teller is configured via environment variables and cannot be changed via the API")
			return
		}

		var req updateTellerConfigRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		appID := strings.TrimSpace(req.ApplicationID)
		if appID == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"application_id is required")
			return
		}

		env := strings.TrimSpace(req.Environment)
		if env == "" {
			env = "sandbox"
		}
		if !validTellerEnvs[env] {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"environment must be one of: sandbox, development, production")
			return
		}

		// Determine the cert/key bytes to persist. Empty / nil values preserve
		// the existing stored cert (admin "leave blank to keep" UX). Note: we
		// deliberately do NOT TrimSpace the PEM bodies — trailing newlines
		// are part of the PEM encoding and trimming them changes the bytes
		// that downstream X.509 parsing sees.
		certPEM := nilSafeString(req.Certificate)
		keyPEM := nilSafeString(req.PrivateKey)
		if strings.TrimSpace(certPEM) == "" {
			certPEM = ""
		}
		if strings.TrimSpace(keyPEM) == "" {
			keyPEM = ""
		}
		switch {
		case certPEM != "" && keyPEM == "":
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"private_key is required when certificate is provided")
			return
		case certPEM == "" && keyPEM != "":
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"certificate is required when private_key is provided")
			return
		}

		var newCertPEM, newKeyPEM []byte
		if certPEM != "" && keyPEM != "" {
			newCertPEM = []byte(certPEM)
			newKeyPEM = []byte(keyPEM)
			if err := tellerprovider.ValidateCredentialsPEM(newCertPEM, newKeyPEM); err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
					"Invalid certificate/key pair: "+err.Error())
				return
			}
			if len(a.Config.EncryptionKey) == 0 {
				mw.WriteError(w, http.StatusInternalServerError, "ENCRYPTION_KEY_MISSING",
					"Encryption key is required to store Teller certificates. Set ENCRYPTION_KEY at startup.")
				return
			}
		}

		// Persist scalar config fields first.
		ctx := r.Context()
		entries := []db.SetAppConfigParams{
			{Key: "teller_app_id", Value: pgconv.Text(appID)},
			{Key: "teller_env", Value: pgconv.Text(env)},
		}
		if req.WebhookSecret != nil {
			trimmed := strings.TrimSpace(*req.WebhookSecret)
			if trimmed != "" {
				entries = append(entries, db.SetAppConfigParams{
					Key: "teller_webhook_secret", Value: pgconv.Text(trimmed),
				})
			}
		}
		for _, entry := range entries {
			if err := a.Queries.SetAppConfig(ctx, entry); err != nil {
				a.Logger.Error("save teller config", "error", err, "key", entry.Key)
				mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
					"Failed to save Teller configuration")
				return
			}
		}
		a.Config.TellerAppID = appID
		a.Config.TellerEnv = env
		if req.WebhookSecret != nil {
			trimmed := strings.TrimSpace(*req.WebhookSecret)
			if trimmed != "" {
				a.Config.TellerWebhookSecret = trimmed
			}
		}
		a.Config.ConfigSources["teller_app_id"] = "db"
		a.Config.ConfigSources["teller_env"] = "db"

		// Persist encrypted cert/key if a new pair was supplied.
		if len(newCertPEM) > 0 && len(newKeyPEM) > 0 {
			if err := storeTellerCert(ctx, a, newCertPEM, newKeyPEM); err != nil {
				a.Logger.Error("store teller certificate", "error", err)
				mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
					"Failed to store Teller certificate: "+err.Error())
				return
			}
			a.Config.TellerCertPEM = newCertPEM
			a.Config.TellerKeyPEM = newKeyPEM
		}

		if err := a.ReinitProvider("teller"); err != nil {
			a.Logger.Error("reinit teller provider", "error", err)
			mw.WriteError(w, http.StatusInternalServerError, "PROVIDER_REINIT_FAILED",
				"Teller configuration saved but provider failed to initialize: "+err.Error())
			return
		}

		writeData(w, buildProviderConfigResponse(a))
	}
}

// storeTellerCert encrypts cert + key with AES-256-GCM, base64-encodes the
// ciphertext, and writes both rows to app_config. Mirrors the storage path
// in internal/admin/providers.go ProvidersSaveTellerHandler.
func storeTellerCert(ctx context.Context, a *app.App, certPEM, keyPEM []byte) error {
	encCert, err := crypto.Encrypt(certPEM, a.Config.EncryptionKey)
	if err != nil {
		return err
	}
	encKey, err := crypto.Encrypt(keyPEM, a.Config.EncryptionKey)
	if err != nil {
		return err
	}
	certB64 := base64.StdEncoding.EncodeToString(encCert)
	keyB64 := base64.StdEncoding.EncodeToString(encKey)
	if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key: "teller_cert_pem", Value: pgconv.Text(certB64),
	}); err != nil {
		return err
	}
	if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key: "teller_key_pem", Value: pgconv.Text(keyB64),
	}); err != nil {
		return err
	}
	return nil
}

// buildProviderConfigResponse renders the redacted view of both providers'
// current configuration. Sensitive fields (secret, certificate body) become
// boolean "*_set" flags — never the raw value.
func buildProviderConfigResponse(a *app.App) providerConfigResponse {
	plaidFromEnv := os.Getenv("PLAID_CLIENT_ID") != ""
	tellerFromEnv := os.Getenv("TELLER_APP_ID") != ""

	plaidConfigured := a.Config.PlaidClientID != "" && a.Config.PlaidSecret != ""
	tellerCertPresent := a.Config.TellerCertPath != "" || (len(a.Config.TellerCertPEM) > 0 && len(a.Config.TellerKeyPEM) > 0)
	tellerConfigured := a.Config.TellerAppID != "" && tellerCertPresent

	return providerConfigResponse{
		Plaid: plaidConfigView{
			Configured:  plaidConfigured,
			FromEnv:     plaidFromEnv,
			ClientID:    a.Config.PlaidClientID,
			Environment: a.Config.PlaidEnv,
			WebhookURL:  a.Config.WebhookURL,
			SecretSet:   a.Config.PlaidSecret != "",
		},
		Teller: tellerConfigView{
			Configured:       tellerConfigured,
			FromEnv:          tellerFromEnv,
			ApplicationID:    a.Config.TellerAppID,
			Environment:      a.Config.TellerEnv,
			CertificateSet:   tellerCertPresent,
			WebhookSecretSet: a.Config.TellerWebhookSecret != "",
		},
	}
}

func stringFromPtr(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// nilSafeString returns *s without any trimming, or "" when s is nil.
func nilSafeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

