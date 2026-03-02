package admin

import (
	"encoding/json"
	"fmt"
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/db"
	plaidprovider "breadbox/internal/provider/plaid"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ConnectionsListHandler serves GET /admin/connections.
func ConnectionsListHandler(a *app.App, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		connections, err := a.Queries.ListBankConnections(ctx)
		if err != nil {
			a.Logger.Error("list bank connections", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		data := map[string]any{
			"PageTitle":   "Connections",
			"CurrentPage": "connections",
			"Connections": connections,
			"CSRFToken":   GetCSRFToken(r),
		}
		tr.Render(w, r, "connections.html", data)
	}
}

// NewConnectionHandler serves GET /admin/connections/new.
func NewConnectionHandler(a *app.App, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		users, err := a.Queries.ListUsers(ctx)
		if err != nil {
			a.Logger.Error("list users", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		data := map[string]any{
			"PageTitle":   "Connect New Bank",
			"CurrentPage": "connections",
			"Users":       users,
			"CSRFToken":   GetCSRFToken(r),
		}
		tr.Render(w, r, "connection_new.html", data)
	}
}

// linkTokenRequest is the JSON body for POST /admin/api/link-token.
type linkTokenRequest struct {
	UserID string `json:"user_id"`
}

// linkTokenResponse is the JSON response for POST /admin/api/link-token.
type linkTokenResponse struct {
	LinkToken  string `json:"link_token"`
	Expiration string `json:"expiration"`
}

// LinkTokenHandler serves POST /admin/api/link-token.
func LinkTokenHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req linkTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		if req.UserID == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "user_id is required"})
			return
		}

		plaidProvider, ok := a.Providers["plaid"]
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Plaid provider not configured"})
			return
		}

		session, err := plaidProvider.CreateLinkSession(r.Context(), req.UserID)
		if err != nil {
			a.Logger.Error("create link session", "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Failed to create link token: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, linkTokenResponse{
			LinkToken:  session.Token,
			Expiration: session.Expiry.Format("2006-01-02T15:04:05Z"),
		})
	}
}

// exchangeTokenRequest is the JSON body for POST /admin/api/exchange-token.
type exchangeTokenRequest struct {
	PublicToken     string            `json:"public_token"`
	UserID          string            `json:"user_id"`
	InstitutionID   string            `json:"institution_id"`
	InstitutionName string            `json:"institution_name"`
	Accounts        []accountMetadata `json:"accounts"`
}

type accountMetadata struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Mask    string `json:"mask"`
}

// exchangeTokenResponse is the JSON response for POST /admin/api/exchange-token.
type exchangeTokenResponse struct {
	ConnectionID    string `json:"connection_id"`
	InstitutionName string `json:"institution_name"`
	Status          string `json:"status"`
}

// ExchangeTokenHandler serves POST /admin/api/exchange-token.
func ExchangeTokenHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req exchangeTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		if req.PublicToken == "" || req.UserID == "" || req.InstitutionID == "" || req.InstitutionName == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Missing required fields"})
			return
		}

		plaidProvider, ok := a.Providers["plaid"]
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Plaid provider not configured"})
			return
		}

		conn, accounts, err := plaidProvider.ExchangeToken(r.Context(), req.PublicToken)
		if err != nil {
			a.Logger.Error("exchange token", "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Failed to exchange token: " + err.Error()})
			return
		}

		// Parse user ID.
		var userID pgtype.UUID
		if err := userID.Scan(req.UserID); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Invalid user_id"})
			return
		}

		// Create the bank connection record.
		bankConn, err := a.Queries.CreateBankConnection(r.Context(), db.CreateBankConnectionParams{
			UserID:           userID,
			Provider:         db.ProviderTypePlaid,
			InstitutionID:    pgtype.Text{String: req.InstitutionID, Valid: true},
			InstitutionName:  pgtype.Text{String: req.InstitutionName, Valid: true},
			PlaidItemID:      pgtype.Text{String: conn.ExternalID, Valid: true},
			PlaidAccessToken: conn.EncryptedCredentials,
			Status:           db.ConnectionStatusActive,
		})
		if err != nil {
			a.Logger.Error("create bank connection", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to save connection"})
			return
		}

		// Upsert accounts from the exchange response.
		for _, acct := range accounts {
			_, err := a.Queries.UpsertAccount(r.Context(), db.UpsertAccountParams{
				ConnectionID:      bankConn.ID,
				ExternalAccountID: acct.ExternalID,
				Name:              acct.Name,
				OfficialName:      pgtype.Text{String: acct.OfficialName, Valid: acct.OfficialName != ""},
				Type:              acct.Type,
				Subtype:           pgtype.Text{String: acct.Subtype, Valid: acct.Subtype != ""},
				Mask:              pgtype.Text{String: acct.Mask, Valid: acct.Mask != ""},
				IsoCurrencyCode:   pgtype.Text{String: acct.ISOCurrencyCode, Valid: acct.ISOCurrencyCode != ""},
			})
			if err != nil {
				a.Logger.Error("upsert account", "error", err, "external_id", acct.ExternalID)
			}
		}

		connID := formatUUID(bankConn.ID)

		writeJSON(w, http.StatusCreated, exchangeTokenResponse{
			ConnectionID:    connID,
			InstitutionName: req.InstitutionName,
			Status:          "active",
		})
	}
}

// ConnectionDetailHandler serves GET /admin/connections/{id}.
func ConnectionDetailHandler(a *app.App, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParam(r, "id")

		var connID pgtype.UUID
		if err := connID.Scan(idStr); err != nil {
			http.Error(w, "Invalid connection ID", http.StatusBadRequest)
			return
		}

		conn, err := a.Queries.GetBankConnection(ctx, connID)
		if err != nil {
			a.Logger.Error("get bank connection", "error", err)
			http.NotFound(w, r)
			return
		}

		accounts, err := a.Queries.ListAccountsByConnection(ctx, connID)
		if err != nil {
			a.Logger.Error("list accounts by connection", "error", err)
		}

		syncLogs, err := a.Queries.GetSyncLogsByConnection(ctx, db.GetSyncLogsByConnectionParams{
			ConnectionID: connID,
			Limit:        10,
		})
		if err != nil {
			a.Logger.Error("get sync logs by connection", "error", err)
		}

		data := map[string]any{
			"PageTitle":   conn.InstitutionName.String,
			"CurrentPage": "connections",
			"Connection":  conn,
			"Accounts":    accounts,
			"SyncLogs":    syncLogs,
			"ConnID":      idStr,
			"CSRFToken":   GetCSRFToken(r),
		}
		tr.Render(w, r, "connection_detail.html", data)
	}
}

// ConnectionReauthHandler serves GET /admin/connections/{id}/reauth.
func ConnectionReauthHandler(a *app.App, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParam(r, "id")

		var connID pgtype.UUID
		if err := connID.Scan(idStr); err != nil {
			http.Error(w, "Invalid connection ID", http.StatusBadRequest)
			return
		}

		conn, err := a.Queries.GetBankConnection(ctx, connID)
		if err != nil {
			a.Logger.Error("get bank connection for reauth", "error", err)
			http.NotFound(w, r)
			return
		}

		data := map[string]any{
			"PageTitle":   "Re-authenticate " + conn.InstitutionName.String,
			"CurrentPage": "connections",
			"Connection":  conn,
			"ConnID":      idStr,
			"CSRFToken":   GetCSRFToken(r),
		}
		tr.Render(w, r, "connection_reauth.html", data)
	}
}

// ConnectionReauthAPIHandler serves POST /admin/api/connections/{id}/reauth.
func ConnectionReauthAPIHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		ctx := r.Context()

		var connID pgtype.UUID
		if err := connID.Scan(idStr); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid connection ID"})
			return
		}

		plaidProv, ok := a.Providers["plaid"].(*plaidprovider.PlaidProvider)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Plaid provider not configured"})
			return
		}

		// Load the connection and decrypt access token.
		conn, err := a.Queries.GetBankConnection(ctx, connID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Connection not found"})
			return
		}

		accessToken, err := plaidprovider.Decrypt(conn.PlaidAccessToken, a.Config.EncryptionKey)
		if err != nil {
			a.Logger.Error("decrypt access token for reauth", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to decrypt access token"})
			return
		}

		session, err := plaidProv.CreateReauthLinkToken(ctx, string(accessToken), formatUUID(conn.UserID))
		if err != nil {
			a.Logger.Error("create reauth link token", "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Failed to create reauth link token"})
			return
		}

		writeJSON(w, http.StatusOK, linkTokenResponse{
			LinkToken:  session.Token,
			Expiration: session.Expiry.Format("2006-01-02T15:04:05Z"),
		})
	}
}

// ConnectionReauthCompleteHandler serves POST /admin/api/connections/{id}/reauth-complete.
func ConnectionReauthCompleteHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")

		var connID pgtype.UUID
		if err := connID.Scan(idStr); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid connection ID"})
			return
		}

		// Update connection status to active and clear errors.
		err := a.Queries.UpdateBankConnectionStatus(r.Context(), db.UpdateBankConnectionStatusParams{
			ID:           connID,
			Status:       db.ConnectionStatusActive,
			ErrorCode:    pgtype.Text{},
			ErrorMessage: pgtype.Text{},
		})
		if err != nil {
			a.Logger.Error("reactivate bank connection", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update connection status"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "active"})
	}
}

// DeleteConnectionHandler serves DELETE /admin/api/connections/{id}.
func DeleteConnectionHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		ctx := r.Context()

		var connID pgtype.UUID
		if err := connID.Scan(idStr); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid connection ID"})
			return
		}

		// Load connection and call Plaid to revoke access token.
		if plaidProv, ok := a.Providers["plaid"].(*plaidprovider.PlaidProvider); ok {
			conn, err := a.Queries.GetBankConnection(ctx, connID)
			if err == nil && conn.PlaidAccessToken != nil {
				accessToken, decErr := plaidprovider.Decrypt(conn.PlaidAccessToken, a.Config.EncryptionKey)
				if decErr == nil {
					_ = plaidProv.RemoveItem(ctx, string(accessToken))
				} else {
					a.Logger.Error("decrypt access token for removal", "error", decErr)
				}
			}
		}

		// Soft-delete the connection locally.
		err := a.Queries.DeleteBankConnection(ctx, connID)
		if err != nil {
			a.Logger.Error("delete bank connection", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete connection"})
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// formatUUID converts a pgtype.UUID to its string representation.
func formatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
