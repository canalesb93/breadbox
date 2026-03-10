package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Webhook event types.
const EventReviewItemsAdded = "review_items_added"

// Webhook delivery statuses.
const (
	DeliveryStatusPending = "pending"
	DeliveryStatusSuccess = "success"
	DeliveryStatusFailed  = "failed"
)

// Retry intervals for failed webhook deliveries.
var retryIntervals = []time.Duration{
	30 * time.Second,
	5 * time.Minute,
	30 * time.Minute,
}

// Dispatcher manages outgoing webhook deliveries with retry logic.
type Dispatcher struct {
	queries *db.Queries
	pool    *pgxpool.Pool
	logger  *slog.Logger
	client  *http.Client
	version string

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
}

// NewDispatcher creates a new outgoing webhook dispatcher.
func NewDispatcher(queries *db.Queries, pool *pgxpool.Pool, logger *slog.Logger, version string) *Dispatcher {
	return &Dispatcher{
		queries: queries,
		pool:    pool,
		logger:  logger,
		version: version,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		stopCh: make(chan struct{}),
	}
}

// Enqueue creates a new delivery record and triggers async delivery.
func (d *Dispatcher) Enqueue(ctx context.Context, event string, payload any, webhookURL string) error {
	if webhookURL == "" {
		return nil // webhooks not configured, skip silently
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	deliveryID := generateUUIDv4()

	_, err = d.queries.InsertWebhookDelivery(ctx, db.InsertWebhookDeliveryParams{
		Event:       event,
		Url:         webhookURL,
		Payload:     payloadBytes,
		DeliveryID:  pgtype.UUID{Bytes: deliveryID, Valid: true},
		NextRetryAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("insert webhook delivery: %w", err)
	}

	// Trigger async processing
	go func() {
		if err := d.ProcessPending(context.Background()); err != nil {
			d.logger.Error("process pending webhooks", "error", err)
		}
	}()

	return nil
}

// ProcessPending processes any pending webhook deliveries.
func (d *Dispatcher) ProcessPending(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return nil // already processing
	}
	d.running = true
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		d.running = false
		d.mu.Unlock()
	}()

	deliveries, err := d.queries.GetPendingWebhookDeliveries(ctx)
	if err != nil {
		return fmt.Errorf("get pending deliveries: %w", err)
	}

	for _, delivery := range deliveries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		d.processDelivery(ctx, delivery)
	}

	return nil
}

// processDelivery attempts to deliver a single webhook.
func (d *Dispatcher) processDelivery(ctx context.Context, delivery db.WebhookDelivery) {
	logger := d.logger.With("delivery_id", formatUUID(delivery.DeliveryID), "event", delivery.Event, "url", delivery.Url)

	// Load the current webhook secret
	var secret string
	if row, err := d.queries.GetAppConfig(ctx, "review_webhook_secret"); err == nil && row.Value.Valid {
		secret = row.Value.String
	}

	// Build the request
	body := delivery.Payload
	timestamp := time.Now().Unix()
	deliveryUUID := formatUUID(delivery.DeliveryID)

	// Compute HMAC signature
	signedPayload := fmt.Sprintf("%d.%s", timestamp, string(body))
	signature := computeHMAC(signedPayload, secret)
	sigHeader := fmt.Sprintf("t=%d,v1=%s", timestamp, signature)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, delivery.Url, bytes.NewReader(body))
	if err != nil {
		logger.Error("create webhook request", "error", err)
		d.markFailed(ctx, delivery, 0, "", err.Error())
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Breadbox/"+d.version)
	req.Header.Set("X-Breadbox-Signature", sigHeader)
	req.Header.Set("X-Breadbox-Event", delivery.Event)
	req.Header.Set("X-Breadbox-Delivery-ID", deliveryUUID)

	resp, err := d.client.Do(req)
	if err != nil {
		logger.Warn("webhook delivery failed", "error", err, "attempt", delivery.Attempts+1)
		d.scheduleRetryOrFail(ctx, delivery, 0, "", err.Error())
		return
	}
	defer resp.Body.Close()

	// Read response body (first 1000 chars)
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1000))
	respBodyStr := string(respBody)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Success
		logger.Info("webhook delivered", "status", resp.StatusCode, "attempt", delivery.Attempts+1)
		d.queries.UpdateWebhookDeliveryAttempt(ctx, db.UpdateWebhookDeliveryAttemptParams{
			ID:             delivery.ID,
			Status:         DeliveryStatusSuccess,
			NextRetryAt:    pgtype.Timestamptz{},
			ResponseStatus: pgtype.Int4{Int32: int32(resp.StatusCode), Valid: true},
			ResponseBody:   pgtype.Text{String: respBodyStr, Valid: respBodyStr != ""},
			ErrorMessage:   pgtype.Text{},
		})
	} else {
		// HTTP error
		logger.Warn("webhook delivery got non-2xx", "status", resp.StatusCode, "attempt", delivery.Attempts+1)
		d.scheduleRetryOrFail(ctx, delivery, resp.StatusCode, respBodyStr, "")
	}
}

// scheduleRetryOrFail either schedules a retry or marks the delivery as failed.
func (d *Dispatcher) scheduleRetryOrFail(ctx context.Context, delivery db.WebhookDelivery, statusCode int, respBody string, errMsg string) {
	attempt := int(delivery.Attempts) // current attempts (0-indexed before this attempt)
	if attempt >= len(retryIntervals) {
		// All retries exhausted
		d.markFailed(ctx, delivery, statusCode, respBody, errMsg)
		return
	}

	nextRetry := time.Now().Add(retryIntervals[attempt])
	d.queries.UpdateWebhookDeliveryAttempt(ctx, db.UpdateWebhookDeliveryAttemptParams{
		ID:             delivery.ID,
		Status:         DeliveryStatusPending,
		NextRetryAt:    pgtype.Timestamptz{Time: nextRetry, Valid: true},
		ResponseStatus: pgtype.Int4{Int32: int32(statusCode), Valid: statusCode > 0},
		ResponseBody:   pgtype.Text{String: respBody, Valid: respBody != ""},
		ErrorMessage:   pgtype.Text{String: errMsg, Valid: errMsg != ""},
	})
}

// markFailed marks a delivery as permanently failed.
func (d *Dispatcher) markFailed(ctx context.Context, delivery db.WebhookDelivery, statusCode int, respBody string, errMsg string) {
	d.logger.Error("webhook delivery failed permanently",
		"delivery_id", formatUUID(delivery.DeliveryID),
		"attempts", delivery.Attempts+1)

	d.queries.UpdateWebhookDeliveryAttempt(ctx, db.UpdateWebhookDeliveryAttemptParams{
		ID:             delivery.ID,
		Status:         DeliveryStatusFailed,
		NextRetryAt:    pgtype.Timestamptz{},
		ResponseStatus: pgtype.Int4{Int32: int32(statusCode), Valid: statusCode > 0},
		ResponseBody:   pgtype.Text{String: respBody, Valid: respBody != ""},
		ErrorMessage:   pgtype.Text{String: errMsg, Valid: errMsg != ""},
	})
}

// Cleanup removes deliveries older than 7 days.
func (d *Dispatcher) Cleanup(ctx context.Context) error {
	return d.queries.CleanupOldWebhookDeliveries(ctx)
}

// SendTestWebhook sends a test webhook to the configured URL and returns the result.
func (d *Dispatcher) SendTestWebhook(ctx context.Context, webhookURL, webhookSecret string) (*TestResult, error) {
	if webhookURL == "" {
		return nil, fmt.Errorf("no webhook URL configured")
	}

	payload := map[string]any{
		"event":     "test",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data": map[string]any{
			"message": "This is a test webhook from Breadbox.",
		},
	}

	body, _ := json.Marshal(payload)
	timestamp := time.Now().Unix()
	deliveryUUID := formatUUID(pgtype.UUID{Bytes: generateUUIDv4(), Valid: true})

	signedPayload := fmt.Sprintf("%d.%s", timestamp, string(body))
	signature := computeHMAC(signedPayload, webhookSecret)
	sigHeader := fmt.Sprintf("t=%d,v1=%s", timestamp, signature)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create test request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Breadbox/"+d.version)
	req.Header.Set("X-Breadbox-Signature", sigHeader)
	req.Header.Set("X-Breadbox-Event", "test")
	req.Header.Set("X-Breadbox-Delivery-ID", deliveryUUID)

	start := time.Now()
	resp, err := d.client.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		return &TestResult{
			Success:        false,
			ResponseTimeMs: int(elapsed.Milliseconds()),
		}, nil
	}
	defer resp.Body.Close()

	return &TestResult{
		Success:        resp.StatusCode >= 200 && resp.StatusCode < 300,
		StatusCode:     resp.StatusCode,
		ResponseTimeMs: int(elapsed.Milliseconds()),
	}, nil
}

// TestResult represents the result of a test webhook delivery.
type TestResult struct {
	Success        bool `json:"success"`
	StatusCode     int  `json:"status_code"`
	ResponseTimeMs int  `json:"response_time_ms"`
}

// computeHMAC computes HMAC-SHA256.
func computeHMAC(message, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// formatUUID formats a pgtype.UUID as a string.
func formatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16])
}

// generateUUIDv4 generates a random UUID v4 as a [16]byte.
func generateUUIDv4() [16]byte {
	var u [16]byte
	_, _ = rand.Read(u[:])
	u[6] = (u[6] & 0x0f) | 0x40 // version 4
	u[8] = (u[8] & 0x3f) | 0x80 // variant 10
	return u
}
