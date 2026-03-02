package webhook

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/provider"
	"breadbox/internal/sync"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// reauthErrorCodes are Plaid error codes that indicate re-authentication is needed.
var reauthErrorCodes = map[string]bool{
	"ITEM_LOGIN_REQUIRED":      true,
	"INSUFFICIENT_CREDENTIALS": true,
	"INVALID_CREDENTIALS":      true,
	"MFA_NOT_SUPPORTED":        true,
	"NO_ACCOUNTS":              true,
	"USER_SETUP_REQUIRED":      true,
}

// NewHandler returns an http.HandlerFunc that processes inbound provider webhooks.
func NewHandler(providers map[string]provider.Provider, engine *sync.Engine, queries *db.Queries, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providerName := chi.URLParam(r, "provider")
		logger := logger.With("provider", providerName)

		prov, ok := providers[providerName]
		if !ok {
			logger.Warn("webhook received for unknown provider")
			writeOK(w)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("failed to read webhook body", "error", err)
			writeOK(w)
			return
		}

		headers := map[string]string{
			"Plaid-Verification": r.Header.Get("Plaid-Verification"),
		}

		event, err := prov.HandleWebhook(r.Context(), provider.WebhookPayload{
			RawBody: body,
			Headers: headers,
		})
		if err != nil {
			logger.Error("webhook verification failed", "error", err)
			writeOK(w)
			return
		}

		logger = logger.With("event_type", event.Type, "item_id", event.ConnectionID)

		// Look up the internal connection by Plaid item ID.
		conn, err := queries.GetBankConnectionByPlaidItemID(r.Context(), pgtype.Text{
			String: event.ConnectionID,
			Valid:  true,
		})
		if err != nil {
			logger.Error("failed to look up connection", "error", err)
			writeOK(w)
			return
		}

		switch event.Type {
		case "sync_available":
			go func() {
				if err := engine.Sync(context.Background(), conn.ID, db.SyncTriggerWebhook); err != nil {
					logger.Error("webhook-triggered sync failed", "error", err)
				}
			}()

		case "connection_error":
			status := db.ConnectionStatusError
			if event.ErrorCode != nil && reauthErrorCodes[*event.ErrorCode] {
				status = db.ConnectionStatusPendingReauth
			}
			var errCode, errMsg pgtype.Text
			if event.ErrorCode != nil {
				errCode = pgtype.Text{String: *event.ErrorCode, Valid: true}
			}
			if event.ErrorMessage != nil {
				errMsg = pgtype.Text{String: *event.ErrorMessage, Valid: true}
			}
			if err := queries.UpdateBankConnectionStatus(r.Context(), db.UpdateBankConnectionStatusParams{
				ID:           conn.ID,
				Status:       status,
				ErrorCode:    errCode,
				ErrorMessage: errMsg,
			}); err != nil {
				logger.Error("failed to update connection status", "error", err)
			}

		case "pending_expiration":
			var expTime pgtype.Timestamptz
			if event.ConsentExpirationTime != nil {
				if t, err := time.Parse(time.RFC3339, *event.ConsentExpirationTime); err == nil {
					expTime = pgtype.Timestamptz{Time: t, Valid: true}
				}
			}
			if err := queries.UpdateConnectionConsentExpiration(r.Context(), db.UpdateConnectionConsentExpirationParams{
				ID:                    conn.ID,
				ConsentExpirationTime: expTime,
			}); err != nil {
				logger.Error("failed to update consent expiration", "error", err)
			}

		case "new_accounts":
			if err := queries.UpdateConnectionNewAccounts(r.Context(), db.UpdateConnectionNewAccountsParams{
				ID:                   conn.ID,
				NewAccountsAvailable: true,
			}); err != nil {
				logger.Error("failed to update new accounts flag", "error", err)
			}

		case "unknown":
			logger.Info("received unknown webhook type, acknowledging")
		}

		writeOK(w)
	}
}

func writeOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
