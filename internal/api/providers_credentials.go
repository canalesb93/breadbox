package api

import (
	"net/http"
	"strings"

	"breadbox/internal/app"
	mw "breadbox/internal/middleware"
	plaidprovider "breadbox/internal/provider/plaid"
	tellerprovider "breadbox/internal/provider/teller"

	"github.com/go-chi/chi/v5"
)

// providerTestResult is the response shape for POST /providers/{name}/test.
type providerTestResult struct {
	Provider string `json:"provider"`
	OK       bool   `json:"ok"`
	Message  string `json:"message,omitempty"`
}

// TestProviderHandler serves POST /api/v1/providers/{name}/test.
//
// It runs a lightweight credentials check against the named provider using
// the currently-configured values (env or app_config), returning {ok:true}
// on success or {ok:false, message: "..."} on failure. The 200/4xx split
// follows the convention used for connection reauth: a reachable server +
// invalid credentials still returns 200 with ok=false so CLI callers can
// branch cleanly without parsing nested error envelopes.
//
// CSV has no credentials and always returns ok=true. Unknown providers
// return 404.
func TestProviderHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "name")))
		if _, ok := providerRegistry[name]; !ok {
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Unknown provider")
			return
		}

		ctx := r.Context()
		result := providerTestResult{Provider: name, OK: true}

		switch name {
		case "plaid":
			if a.Config.PlaidClientID == "" || a.Config.PlaidSecret == "" {
				result.OK = false
				result.Message = "Plaid is not configured"
				writeJSON(w, http.StatusOK, result)
				return
			}
			if err := plaidprovider.ValidateCredentials(ctx, a.Config.PlaidClientID, a.Config.PlaidSecret, a.Config.PlaidEnv); err != nil {
				result.OK = false
				result.Message = err.Error()
			}
		case "teller":
			if a.Config.TellerAppID == "" {
				result.OK = false
				result.Message = "Teller is not configured"
				writeJSON(w, http.StatusOK, result)
				return
			}
			switch {
			case a.Config.TellerCertPath != "" && a.Config.TellerKeyPath != "":
				if err := tellerprovider.ValidateCredentials(a.Config.TellerCertPath, a.Config.TellerKeyPath); err != nil {
					result.OK = false
					result.Message = err.Error()
				}
			case len(a.Config.TellerCertPEM) > 0 && len(a.Config.TellerKeyPEM) > 0:
				if err := tellerprovider.ValidateCredentialsPEM(a.Config.TellerCertPEM, a.Config.TellerKeyPEM); err != nil {
					result.OK = false
					result.Message = err.Error()
				}
			default:
				result.OK = false
				result.Message = "Teller certificate is not configured"
			}
		case "csv":
			// CSV needs no credentials; nothing to check.
		}

		writeJSON(w, http.StatusOK, result)
	}
}

// DisableProviderHandler serves DELETE /api/v1/providers/{name}.
//
// "Disable" means: remove the credentials from app_config (and the in-memory
// config struct), then re-run the live provider init so the map drops the
// provider. Existing connections stay in the DB and continue to surface in
// listings, but sync attempts fail with a "provider not configured" error
// until credentials are restored.
//
// CSV is always available — it has no credentials to clear — so disabling
// CSV is a no-op that returns 200.
func DisableProviderHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "name")))
		if _, ok := providerRegistry[name]; !ok {
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Unknown provider")
			return
		}
		ctx := r.Context()
		switch name {
		case "plaid":
			for _, k := range []string{"plaid_client_id", "plaid_secret", "plaid_env"} {
				_ = a.Queries.DeleteAppConfig(ctx, k)
				if a.Config.ConfigSources != nil {
					delete(a.Config.ConfigSources, k)
				}
			}
			a.Config.PlaidClientID = ""
			a.Config.PlaidSecret = ""
			a.Config.PlaidEnv = ""
			if err := a.ReinitProvider("plaid"); err != nil {
				a.Logger.Error("disable plaid", "error", err)
				mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
					"Failed to disable Plaid")
				return
			}
		case "teller":
			for _, k := range []string{"teller_app_id", "teller_env", "teller_webhook_secret", "teller_cert_pem", "teller_key_pem"} {
				_ = a.Queries.DeleteAppConfig(ctx, k)
				if a.Config.ConfigSources != nil {
					delete(a.Config.ConfigSources, k)
				}
			}
			a.Config.TellerAppID = ""
			a.Config.TellerEnv = ""
			a.Config.TellerWebhookSecret = ""
			a.Config.TellerCertPEM = nil
			a.Config.TellerKeyPEM = nil
			if err := a.ReinitProvider("teller"); err != nil {
				a.Logger.Error("disable teller", "error", err)
				mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
					"Failed to disable Teller")
				return
			}
		case "csv":
			// no-op
		}
		writeJSON(w, http.StatusOK, providerTestResult{Provider: name, OK: true, Message: "disabled"})
	}
}
