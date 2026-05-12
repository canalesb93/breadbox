package api

// Teller entry in the generic provider dispatch table. Mirrors the legacy
// POST /connections/teller body (minus the top-level user_id, which is on
// the outer envelope of POST /connections).

import (
	"encoding/json"
	"net/http"

	"breadbox/internal/app"
	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/jackc/pgx/v5/pgtype"
)

// tellerCredentials is the JSON shape required under `credentials` for
// POST /api/v1/connections with provider:"teller".
type tellerCredentials struct {
	AccessToken     string                 `json:"access_token"`
	EnrollmentID    string                 `json:"enrollment_id"`
	InstitutionID   string                 `json:"institution_id"`
	InstitutionName string                 `json:"institution_name"`
	Accounts        []tellerSetupAccountMD `json:"accounts"`
}

var tellerEntry = providerEntry{
	name:             "teller",
	needsLinkSession: false, // Teller Connect runs entirely client-side; no init token
	capabilities:     []string{"transactions", "balances"},
	credentialsSchema: map[string]CredentialField{
		"access_token": {
			Type:        "string",
			Required:    true,
			Description: "Long-lived access token returned by Teller Connect's onSuccess callback",
		},
		"enrollment_id": {
			Type:        "string",
			Required:    true,
			Description: "Teller enrollment identifier (e.g. enr_abc123)",
		},
		"institution_id": {
			Type:        "string",
			Required:    false,
			Description: "Teller institution identifier (optional; not always provided by Teller)",
		},
		"institution_name": {
			Type:        "string",
			Required:    true,
			Description: "Human-readable institution name",
		},
		"accounts": {
			Type:        "array",
			Required:    false,
			Description: "Informational; accounts persisted come from the provider's exchange response",
		},
	},
	extractFromJSON: func(w http.ResponseWriter, raw json.RawMessage) any {
		var creds tellerCredentials
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &creds); err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY",
					"credentials must be a JSON object")
				return nil
			}
		}
		if creds.AccessToken == "" || creds.EnrollmentID == "" || creds.InstitutionName == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"credentials.access_token, credentials.enrollment_id, and credentials.institution_name are required")
			return nil
		}
		return creds
	},
	exchange: func(a *app.App, w http.ResponseWriter, r *http.Request, uid pgtype.UUID, raw any) {
		creds := raw.(tellerCredentials)
		ctx := r.Context()

		prov, ok := a.Providers["teller"]
		if !ok {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"Teller provider is not configured on this server")
			return
		}

		// Teller's ExchangeToken signature takes a `publicToken` string; the
		// provider treats it as an opaque JSON blob carrying access_token +
		// enrollment_id + institution_name. Build the same blob the legacy
		// handler does so persistence is byte-identical.
		blob, err := json.Marshal(tellerExchangeBlob{
			AccessToken:     creds.AccessToken,
			EnrollmentID:    creds.EnrollmentID,
			InstitutionName: creds.InstitutionName,
		})
		if err != nil {
			a.Logger.Error("encode teller exchange blob", "error", err)
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to prepare provider call")
			return
		}

		conn, accounts, err := prov.ExchangeToken(ctx, string(blob))
		if err != nil {
			a.Logger.Error("exchange teller enrollment", "error", err)
			mw.WriteError(w, http.StatusBadGateway, "PROVIDER_ERROR", "Failed to exchange enrollment payload")
			return
		}

		result, err := a.Service.RegisterNewConnection(ctx, service.RegisterNewConnectionParams{
			UserID:          uid,
			Provider:        "teller",
			InstitutionID:   creds.InstitutionID,
			InstitutionName: creds.InstitutionName,
			Conn:            conn,
			Accounts:        accounts,
		})
		if err != nil {
			a.Logger.Error("register teller connection", "error", err)
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to save connection")
			return
		}

		writeJSON(w, http.StatusCreated, connectionEnvelope{
			ConnectionID:    result.ShortID,
			InstitutionName: creds.InstitutionName,
			Status:          "active",
		})
	},
}
