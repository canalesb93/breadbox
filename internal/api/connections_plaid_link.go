//go:build !lite

package api

import (
	"errors"
	"net/http"

	"breadbox/internal/app"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
)

// plaidLinkTokenRequest is the JSON body for POST /api/v1/connections/plaid/link-token.
type plaidLinkTokenRequest struct {
	UserID string `json:"user_id"`
}

// plaidLinkTokenResponse mirrors the admin linkTokenResponse so the SDK
// surface is uniform across reauth (api.reauthLinkTokenResponse) and
// new-connection link-token issuance.
type plaidLinkTokenResponse struct {
	LinkToken  string `json:"link_token"`
	Expiration string `json:"expiration"`
}

// plaidExchangeRequest is the JSON body for POST /api/v1/connections/plaid/exchange.
//
// Mirrors the admin exchangeTokenRequest minus the `provider` discriminator —
// this endpoint is Plaid-specific. The `accounts` slice is the metadata
// Plaid Link's onSuccess callback returns; it is informational only —
// persistence uses the accounts the provider returns from ExchangeToken.
type plaidExchangeRequest struct {
	PublicToken     string                `json:"public_token"`
	UserID          string                `json:"user_id"`
	InstitutionID   string                `json:"institution_id"`
	InstitutionName string                `json:"institution_name"`
	Accounts        []plaidExchangeAcctMD `json:"accounts"`
}

type plaidExchangeAcctMD struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Mask    string `json:"mask"`
}

// plaidExchangeResponse is the JSON response for POST /api/v1/connections/plaid/exchange.
type plaidExchangeResponse struct {
	ConnectionID    string `json:"connection_id"`
	InstitutionName string `json:"institution_name"`
	Status          string `json:"status"`
}

// PlaidLinkTokenHandler serves POST /api/v1/connections/plaid/link-token.
// Returns a fresh Plaid link token that the host can hand to Plaid Link to
// start a new bank connection. Mirrors the admin LinkTokenHandler but
// speaks the standard REST error envelope and accepts short_id alongside
// UUID for user_id.
func PlaidLinkTokenHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var req plaidLinkTokenRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.UserID == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "user_id is required")
			return
		}

		// Validate the user exists before paying the provider round trip.
		uid, err := a.Service.ResolveUserUUID(ctx, req.UserID)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "User not found")
				return
			}
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "Invalid user_id")
			return
		}

		prov, ok := a.Providers["plaid"]
		if !ok {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"Plaid provider is not configured on this server")
			return
		}

		// Plaid expects the host's user identifier as a string; pass the
		// canonical UUID so retries land on the same Plaid `client_user_id`
		// regardless of whether the caller used a short_id at the API edge.
		session, err := prov.CreateLinkSession(ctx, pgconv.FormatUUID(uid))
		if err != nil {
			a.Logger.Error("create plaid link session", "error", err)
			mw.WriteError(w, http.StatusBadGateway, "PROVIDER_ERROR", "Failed to create link token")
			return
		}

		writeJSON(w, http.StatusOK, plaidLinkTokenResponse{
			LinkToken:  session.Token,
			Expiration: session.Expiry.Format("2006-01-02T15:04:05Z"),
		})
	}
}

// PlaidExchangeHandler serves POST /api/v1/connections/plaid/exchange.
// Exchanges the public token Plaid returned in Link's onSuccess for a
// stored BankConnection and the accounts Plaid returned alongside it.
//
// The request `accounts` field is informational; the rows persisted come
// from the provider's ExchangeToken response so the DB matches what Plaid
// authoritatively reports.
func PlaidExchangeHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var req plaidExchangeRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.PublicToken == "" || req.UserID == "" || req.InstitutionID == "" || req.InstitutionName == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"public_token, user_id, institution_id, and institution_name are required")
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

		prov, ok := a.Providers["plaid"]
		if !ok {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"Plaid provider is not configured on this server")
			return
		}

		conn, accounts, err := prov.ExchangeToken(ctx, req.PublicToken)
		if err != nil {
			a.Logger.Error("exchange plaid public token", "error", err)
			mw.WriteError(w, http.StatusBadGateway, "PROVIDER_ERROR", "Failed to exchange public token")
			return
		}

		result, err := a.Service.RegisterNewConnection(ctx, service.RegisterNewConnectionParams{
			UserID:          uid,
			Provider:        "plaid",
			InstitutionID:   req.InstitutionID,
			InstitutionName: req.InstitutionName,
			Conn:            conn,
			Accounts:        accounts,
		})
		if err != nil {
			a.Logger.Error("register plaid connection", "error", err)
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to save connection")
			return
		}

		writeJSON(w, http.StatusCreated, plaidExchangeResponse{
			ConnectionID:    result.ShortID,
			InstitutionName: req.InstitutionName,
			Status:          "active",
		})
	}
}
