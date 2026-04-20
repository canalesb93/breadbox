package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/provider"
	"breadbox/internal/sync"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// NewHandler returns an http.HandlerFunc that processes inbound provider webhooks.
func NewHandler(providers map[string]provider.Provider, engine *sync.Engine, queries *db.Queries, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providerName := chi.URLParam(r, "provider")
		logger := logger.With("provider", providerName)

		prov, ok := providers[providerName]
		if !ok {
			logger.Warn("webhook received for unknown provider")
			// Log even for unknown providers so we can see unexpected traffic.
			logWebhookEvent(r.Context(), queries, logger, db.ProviderType(providerName), "unknown", pgtype.UUID{}, nil, "error", strPtr("unknown provider: "+providerName))
			writeOK(w)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("failed to read webhook body", "error", err)
			writeOK(w)
			return
		}

		// Compute SHA-256 hash of the raw payload for dedup and auditing.
		payloadHash := sha256Hex(body)

		headers := make(map[string]string)
		for key, vals := range r.Header {
			if len(vals) > 0 {
				headers[key] = vals[0]
			}
		}

		event, err := prov.HandleWebhook(r.Context(), provider.WebhookPayload{
			RawBody: body,
			Headers: headers,
		})
		if err != nil {
			logger.Error("webhook verification failed", "error", err)
			logWebhookEvent(r.Context(), queries, logger, db.ProviderType(providerName), "verification_failed", pgtype.UUID{}, &payloadHash, "error", strPtr("verification failed: "+err.Error()))
			writeOK(w)
			return
		}

		logger = logger.With("event_type", event.Type, "item_id", event.ConnectionID)

		// Look up the internal connection by provider + external ID.
		conn, err := queries.GetBankConnectionByExternalID(r.Context(), db.GetBankConnectionByExternalIDParams{
			Provider:   db.ProviderType(providerName),
			ExternalID: pgconv.Text(event.ConnectionID),
		})
		if err != nil {
			logger.Error("failed to look up connection", "error", err)
			logWebhookEvent(r.Context(), queries, logger, db.ProviderType(providerName), event.Type, pgtype.UUID{}, &payloadHash, "error", strPtr("connection lookup failed: "+err.Error()))
			writeOK(w)
			return
		}

		// Create the webhook event log entry before processing.
		webhookEvent := logWebhookEvent(r.Context(), queries, logger, db.ProviderType(providerName), event.Type, conn.ID, &payloadHash, "received", nil)

		var processErr error
		switch event.Type {
		case "sync_available":
			go func() {
				if err := engine.Sync(context.Background(), conn.ID, db.SyncTriggerWebhook); err != nil {
					logger.Error("webhook-triggered sync failed", "error", err)
					// Update event status asynchronously on failure.
					updateWebhookEventStatus(context.Background(), queries, logger, webhookEvent, "error", strPtr("sync failed: "+err.Error()))
				} else {
					updateWebhookEventStatus(context.Background(), queries, logger, webhookEvent, "processed", nil)
				}
			}()
			// Don't mark as processed here since sync runs in background.
			writeOK(w)
			return

		case "connection_error":
			status := db.ConnectionStatusError
			if event.NeedsReauth {
				status = db.ConnectionStatusPendingReauth
			}
			var errCode, errMsg pgtype.Text
			if event.ErrorCode != nil {
				errCode = pgtype.Text{String: *event.ErrorCode, Valid: true}
			}
			if event.ErrorMessage != nil {
				errMsg = pgtype.Text{String: *event.ErrorMessage, Valid: true}
			}
			processErr = queries.UpdateBankConnectionStatus(r.Context(), db.UpdateBankConnectionStatusParams{
				ID:           conn.ID,
				Status:       status,
				ErrorCode:    errCode,
				ErrorMessage: errMsg,
			})
			if processErr != nil {
				logger.Error("failed to update connection status", "error", processErr)
			}

		case "pending_expiration":
			var expTime pgtype.Timestamptz
			if event.ConsentExpirationTime != nil {
				if t, err := time.Parse(time.RFC3339, *event.ConsentExpirationTime); err == nil {
					expTime = pgtype.Timestamptz{Time: t, Valid: true}
				}
			}
			processErr = queries.UpdateConnectionConsentExpiration(r.Context(), db.UpdateConnectionConsentExpirationParams{
				ID:                    conn.ID,
				ConsentExpirationTime: expTime,
			})
			if processErr != nil {
				logger.Error("failed to update consent expiration", "error", processErr)
			}

		case "new_accounts":
			processErr = queries.UpdateConnectionNewAccounts(r.Context(), db.UpdateConnectionNewAccountsParams{
				ID:                   conn.ID,
				NewAccountsAvailable: true,
			})
			if processErr != nil {
				logger.Error("failed to update new accounts flag", "error", processErr)
			}

		case "unknown":
			logger.Info("received unknown webhook type, acknowledging")
		}

		// Update the webhook event status based on processing outcome.
		if processErr != nil {
			updateWebhookEventStatus(r.Context(), queries, logger, webhookEvent, "error", strPtr(processErr.Error()))
		} else {
			updateWebhookEventStatus(r.Context(), queries, logger, webhookEvent, "processed", nil)
		}

		writeOK(w)
	}
}

// logWebhookEvent creates a webhook_events record. Returns the event ID (zero-value UUID if logging fails).
func logWebhookEvent(ctx context.Context, queries *db.Queries, logger *slog.Logger, providerType db.ProviderType, eventType string, connectionID pgtype.UUID, payloadHash *string, status string, errMsg *string) pgtype.UUID {
	hash := ""
	if payloadHash != nil {
		hash = *payloadHash
	}
	var errText pgtype.Text
	if errMsg != nil {
		errText = pgtype.Text{String: *errMsg, Valid: true}
	}
	evt, err := queries.CreateWebhookEvent(ctx, db.CreateWebhookEventParams{
		Provider:       providerType,
		EventType:      eventType,
		ConnectionID:   connectionID,
		RawPayloadHash: hash,
		Status:         status,
		ErrorMessage:   errText,
	})
	if err != nil {
		// Don't fail the webhook handler if event logging fails.
		logger.Warn("failed to log webhook event", "error", err)
		return pgtype.UUID{}
	}
	return evt.ID
}

// updateWebhookEventStatus updates the status of a previously logged webhook event.
func updateWebhookEventStatus(ctx context.Context, queries *db.Queries, logger *slog.Logger, eventID pgtype.UUID, status string, errMsg *string) {
	if !eventID.Valid {
		return // Event logging failed earlier, skip update.
	}
	var errText pgtype.Text
	if errMsg != nil {
		errText = pgtype.Text{String: *errMsg, Valid: true}
	}
	if err := queries.UpdateWebhookEventStatus(ctx, db.UpdateWebhookEventStatusParams{
		ID:           eventID,
		Status:       status,
		ErrorMessage: errText,
	}); err != nil {
		logger.Warn("failed to update webhook event status", "event_id", fmt.Sprintf("%x", eventID.Bytes), "error", err)
	}
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func strPtr(s string) *string {
	return &s
}

func writeOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
