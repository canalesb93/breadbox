//go:build !lite

package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"breadbox/internal/app"
	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
)

// tellerSetupRequest is the JSON body for POST /api/v1/connections/teller.
//
// Teller's enrollment flow is materially different from Plaid's: Teller
// Connect runs entirely client-side without a server-issued init token, so
// there's no `link-token` step. The host runs Teller Connect, the user
// enrolls with their bank, and Teller's onSuccess callback hands back an
// `enrollment` payload (access_token + enrollment.id + institution + the
// accounts it discovered). Forward that payload here to register the
// connection.
//
// The `accounts` slice is informational only — the rows persisted come
// from the provider's `ExchangeToken` response (which calls Teller's
// `GET /accounts` with the freshly-issued access token), matching the
// behavior of POST /connections/plaid/exchange.
type tellerSetupRequest struct {
	UserID          string                 `json:"user_id"`
	InstitutionID   string                 `json:"institution_id"`
	InstitutionName string                 `json:"institution_name"`
	AccessToken     string                 `json:"access_token"`
	EnrollmentID    string                 `json:"enrollment_id"`
	Accounts        []tellerSetupAccountMD `json:"accounts"`
}

type tellerSetupAccountMD struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Subtype  string `json:"subtype"`
	LastFour string `json:"last_four"`
}

// tellerSetupResponse mirrors plaidExchangeResponse so the SDK surface stays
// uniform across providers.
type tellerSetupResponse struct {
	ConnectionID    string `json:"connection_id"`
	InstitutionName string `json:"institution_name"`
	Status          string `json:"status"`
}

// tellerExchangeBlob is the JSON shape provider/teller.ExchangeToken
// expects as its `publicToken` argument. Keeping this private to the
// handler keeps the service layer provider-agnostic — the encoding
// detail belongs at the HTTP boundary, not in the persistence path.
type tellerExchangeBlob struct {
	AccessToken     string `json:"access_token"`
	EnrollmentID    string `json:"enrollment_id"`
	InstitutionName string `json:"institution_name"`
}

// TellerSetupHandler serves POST /api/v1/connections/teller.
//
// Registers a Teller connection from the enrollment payload Teller Connect
// returns on success. Builds the access_token + enrollment_id blob the
// Teller provider's `ExchangeToken` accepts, then reuses
// `service.RegisterNewConnection` (shared with the admin handler and
// PlaidExchangeHandler) for persistence so all three surfaces leave the
// DB in identical shape.
func TellerSetupHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var req tellerSetupRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.UserID == "" || req.InstitutionName == "" || req.AccessToken == "" || req.EnrollmentID == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"user_id, institution_name, access_token, and enrollment_id are required")
			return
		}

		uid, err := a.Service.ResolveUserUUID(ctx, req.UserID)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "User not found")
				return
			}
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "Invalid user_id")
			return
		}

		prov, ok := a.Providers["teller"]
		if !ok {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"Teller provider is not configured on this server")
			return
		}

		blob, err := buildTellerExchangeBlob(req)
		if err != nil {
			// Should be unreachable — JSON marshal of a fixed struct
			// of strings doesn't fail in practice. Treat as 500 for safety.
			a.Logger.Error("encode teller exchange blob", "error", err)
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to prepare provider call")
			return
		}

		conn, accounts, err := prov.ExchangeToken(ctx, blob)
		if err != nil {
			a.Logger.Error("exchange teller enrollment", "error", err)
			mw.WriteError(w, http.StatusBadGateway, "PROVIDER_ERROR", "Failed to exchange enrollment payload")
			return
		}

		result, err := a.Service.RegisterNewConnection(ctx, service.RegisterNewConnectionParams{
			UserID:          uid,
			Provider:        "teller",
			InstitutionID:   req.InstitutionID,
			InstitutionName: req.InstitutionName,
			Conn:            conn,
			Accounts:        accounts,
		})
		if err != nil {
			a.Logger.Error("register teller connection", "error", err)
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to save connection")
			return
		}

		writeJSON(w, http.StatusCreated, tellerSetupResponse{
			ConnectionID:    result.ShortID,
			InstitutionName: req.InstitutionName,
			Status:          "active",
		})
	}
}

// buildTellerExchangeBlob serializes the access_token + enrollment_id +
// institution_name triple into the JSON string the Teller provider's
// ExchangeToken accepts. Lives here (not in the service layer) so the
// service stays oblivious to Teller-specific encoding.
func buildTellerExchangeBlob(req tellerSetupRequest) (string, error) {
	b, err := json.Marshal(tellerExchangeBlob{
		AccessToken:     req.AccessToken,
		EnrollmentID:    req.EnrollmentID,
		InstitutionName: req.InstitutionName,
	})
	if err != nil {
		return "", err
	}
	return string(b), nil
}
