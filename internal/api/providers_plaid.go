package api

// Plaid entry in the generic provider dispatch table. The JSON-credentials
// shape matches what POST /connections/plaid/exchange has accepted since
// Bundle 8 — same field names, same semantics — so the deprecated route
// can be implemented as a thin pass-through.

import (
	"encoding/json"
	"net/http"

	"breadbox/internal/app"
	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/jackc/pgx/v5/pgtype"
)

// plaidCredentials is the JSON shape required under `credentials` for
// POST /api/v1/connections with provider:"plaid". Mirrors the legacy
// plaidExchangeRequest fields one-to-one (minus user_id, which lives at
// the top level of the generic body).
type plaidCredentials struct {
	PublicToken     string                `json:"public_token"`
	InstitutionID   string                `json:"institution_id"`
	InstitutionName string                `json:"institution_name"`
	Accounts        []plaidExchangeAcctMD `json:"accounts"`
}

var plaidEntry = providerEntry{
	name:             "plaid",
	needsLinkSession: true,
	capabilities:     []string{"transactions", "balances"},
	credentialsSchema: map[string]CredentialField{
		"public_token": {
			Type:        "string",
			Required:    true,
			Description: "One-time public token returned by Plaid Link's onSuccess callback",
		},
		"institution_id": {
			Type:        "string",
			Required:    true,
			Description: "Plaid institution identifier (e.g. ins_109511)",
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
		var creds plaidCredentials
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &creds); err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY",
					"credentials must be a JSON object")
				return nil
			}
		}
		if creds.PublicToken == "" || creds.InstitutionID == "" || creds.InstitutionName == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"credentials.public_token, credentials.institution_id, and credentials.institution_name are required")
			return nil
		}
		return creds
	},
	exchange: func(a *app.App, w http.ResponseWriter, r *http.Request, uid pgtype.UUID, raw any) {
		creds := raw.(plaidCredentials)
		ctx := r.Context()

		prov, ok := a.Providers["plaid"]
		if !ok {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"Plaid provider is not configured on this server")
			return
		}

		conn, accounts, err := prov.ExchangeToken(ctx, creds.PublicToken)
		if err != nil {
			a.Logger.Error("exchange plaid public token", "error", err)
			mw.WriteError(w, http.StatusBadGateway, "PROVIDER_ERROR", "Failed to exchange public token")
			return
		}

		result, err := a.Service.RegisterNewConnection(ctx, service.RegisterNewConnectionParams{
			UserID:          uid,
			Provider:        "plaid",
			InstitutionID:   creds.InstitutionID,
			InstitutionName: creds.InstitutionName,
			Conn:            conn,
			Accounts:        accounts,
		})
		if err != nil {
			a.Logger.Error("register plaid connection", "error", err)
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
