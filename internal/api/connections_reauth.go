package api

import (
	"errors"
	"net/http"

	"breadbox/internal/app"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	"breadbox/internal/provider"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// reauthLinkTokenResponse is the JSON shape returned by
// POST /api/v1/connections/{id}/reauth. Mirrors the admin
// linkTokenResponse field-for-field so SDK clients see one contract for
// link-token issuance regardless of which surface called it.
type reauthLinkTokenResponse struct {
	LinkToken  string `json:"link_token"`
	Expiration string `json:"expiration"`
}

// ConnectionReauthHandler serves POST /api/v1/connections/{id}/reauth.
// Mirrors the admin ConnectionReauthAPIHandler but speaks the standard REST
// error envelope and accepts short_id alongside UUID. Takes *app.App
// directly because the service layer does not carry the provider registry.
func ConnectionReauthHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := chi.URLParam(r, "id")

		uid, err := a.Service.ResolveConnectionUUID(ctx, id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Connection not found")
				return
			}
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "Invalid connection id")
			return
		}

		conn, err := a.Queries.GetBankConnection(ctx, uid)
		if err != nil {
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Connection not found")
			return
		}

		prov, ok := a.Providers[string(conn.Provider)]
		if !ok {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"Provider "+string(conn.Provider)+" is not configured")
			return
		}

		provConn := provider.Connection{
			ProviderName:         string(conn.Provider),
			ExternalID:           conn.ExternalID.String,
			EncryptedCredentials: conn.EncryptedCredentials,
			UserID:               pgconv.FormatUUID(conn.UserID),
		}

		session, err := prov.CreateReauthSession(ctx, provConn)
		if err != nil {
			a.Logger.Error("create reauth session", "error", err, "connection_id", pgconv.FormatUUID(uid))
			mw.WriteError(w, http.StatusBadGateway, "PROVIDER_ERROR", "Failed to create reauth link token")
			return
		}

		writeJSON(w, http.StatusOK, reauthLinkTokenResponse{
			LinkToken:  session.Token,
			Expiration: session.Expiry.Format("2006-01-02T15:04:05Z"),
		})
	}
}

// ConnectionReauthCompleteHandler serves
// POST /api/v1/connections/{id}/reauth-complete. Marks the connection
// active again after the user finished the provider's re-auth flow.
//
// The body is intentionally ignored — the admin handler takes none either,
// and Plaid's link/oauth-redirect path completes the token exchange
// out-of-band. Future provider flows that need a public_token can extend the
// payload backwards-compatibly.
func ConnectionReauthCompleteHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		actor := service.ActorFromContext(r.Context())

		if err := a.Service.ReactivateConnection(r.Context(), id, actor); err != nil {
			writeServiceError(w, err, "Connection not found", "Failed to reactivate connection")
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "active"})
	}
}
