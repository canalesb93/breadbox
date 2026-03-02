package teller

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/provider"
)

// tellerWebhookBody represents the JSON structure of a Teller webhook.
type tellerWebhookBody struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Payload   struct {
		EnrollmentID string `json:"enrollment_id"`
		Reason       string `json:"reason"`
	} `json:"payload"`
}

// HandleWebhook verifies and parses an inbound Teller webhook.
func (p *TellerProvider) HandleWebhook(ctx context.Context, payload provider.WebhookPayload) (provider.WebhookEvent, error) {
	sigHeader := payload.Headers["Teller-Signature"]
	if sigHeader == "" {
		return provider.WebhookEvent{}, fmt.Errorf("teller: missing Teller-Signature header")
	}

	if err := p.verifySignature(sigHeader, payload.RawBody); err != nil {
		return provider.WebhookEvent{}, err
	}

	var body tellerWebhookBody
	if err := json.Unmarshal(payload.RawBody, &body); err != nil {
		return provider.WebhookEvent{}, fmt.Errorf("teller: invalid webhook body: %w", err)
	}

	event := provider.WebhookEvent{
		ConnectionID: body.Payload.EnrollmentID,
	}

	switch body.Type {
	case "enrollment.disconnected":
		event.Type = "connection_error"
		event.NeedsReauth = true
		if body.Payload.Reason != "" {
			event.ErrorCode = &body.Payload.Reason
		}
	case "transactions.processed":
		event.Type = "sync_available"
	default:
		event.Type = "unknown"
	}

	return event, nil
}

// verifySignature parses the Teller-Signature header, verifies the HMAC-SHA256
// signature, and checks for replay attacks.
//
// Header format: t=1688960969,v1=signature1,v1=signature2
func (p *TellerProvider) verifySignature(header string, body []byte) error {
	parts := strings.Split(header, ",")

	var timestamp string
	var signatures []string

	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			signatures = append(signatures, kv[1])
		}
	}

	if timestamp == "" {
		return fmt.Errorf("teller: missing timestamp in signature header")
	}
	if len(signatures) == 0 {
		return fmt.Errorf("teller: missing v1 signature in signature header")
	}

	// Replay protection: reject if timestamp is older than 5 minutes.
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("teller: invalid timestamp: %w", err)
	}
	age := math.Abs(float64(time.Now().Unix() - ts))
	if age > 300 {
		return fmt.Errorf("teller: webhook timestamp too old (%d seconds)", int(age))
	}

	// Compute expected signature: HMAC-SHA256("{timestamp}.{body}")
	signedMessage := timestamp + "." + string(body)
	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	mac.Write([]byte(signedMessage))
	expected := mac.Sum(nil)

	// Compare against each v1= value (supports key rotation).
	for _, sig := range signatures {
		decoded, err := hex.DecodeString(sig)
		if err != nil {
			continue
		}
		if hmac.Equal(expected, decoded) {
			return nil
		}
	}

	return fmt.Errorf("teller: webhook signature verification failed")
}
