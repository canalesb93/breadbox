package plaid

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"breadbox/internal/provider"

	"github.com/golang-jwt/jwt/v5"
	plaidgo "github.com/plaid/plaid-go/v29/plaid"
)

// handleWebhook verifies the Plaid webhook JWT signature and parses the event.
func (p *PlaidProvider) handleWebhook(ctx context.Context, payload provider.WebhookPayload) (provider.WebhookEvent, error) {
	// Step 1: Extract JWT from Plaid-Verification header.
	token := payload.Headers["Plaid-Verification"]
	if token == "" {
		return provider.WebhookEvent{}, fmt.Errorf("missing Plaid-Verification header")
	}

	// Step 2: Parse JWT header without verification to get alg and kid.
	parser := jwt.NewParser()
	unverified, _, err := parser.ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return provider.WebhookEvent{}, fmt.Errorf("parse JWT header: %w", err)
	}

	// Step 3: Reject if alg is not ES256.
	if unverified.Method.Alg() != "ES256" {
		return provider.WebhookEvent{}, fmt.Errorf("unexpected JWT algorithm: %s", unverified.Method.Alg())
	}

	kid, ok := unverified.Header["kid"].(string)
	if !ok || kid == "" {
		return provider.WebhookEvent{}, fmt.Errorf("missing kid in JWT header")
	}

	// Step 4: Get or fetch the public key for this kid.
	pubKey, err := p.getPublicKey(ctx, kid)
	if err != nil {
		return provider.WebhookEvent{}, fmt.Errorf("get public key: %w", err)
	}

	// Step 5-6: Verify JWT signature with the public key.
	claims := jwt.MapClaims{}
	verified, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pubKey, nil
	})
	if err != nil || !verified.Valid {
		return provider.WebhookEvent{}, fmt.Errorf("JWT verification failed: %w", err)
	}

	// Step 7: Validate iat claim is within 5 minutes.
	iatVal, ok := claims["iat"]
	if !ok {
		return provider.WebhookEvent{}, fmt.Errorf("missing iat claim")
	}
	var iat float64
	switch v := iatVal.(type) {
	case float64:
		iat = v
	case json.Number:
		iat, err = v.Float64()
		if err != nil {
			return provider.WebhookEvent{}, fmt.Errorf("invalid iat claim: %w", err)
		}
	default:
		return provider.WebhookEvent{}, fmt.Errorf("invalid iat claim type: %T", iatVal)
	}
	issuedAt := time.Unix(int64(iat), 0)
	if time.Since(issuedAt) > 5*time.Minute {
		return provider.WebhookEvent{}, fmt.Errorf("JWT iat too old: %s", issuedAt)
	}

	// Step 8: Verify request_body_sha256 matches SHA-256 of raw body.
	bodySHA256Claim, ok := claims["request_body_sha256"].(string)
	if !ok {
		return provider.WebhookEvent{}, fmt.Errorf("missing request_body_sha256 claim")
	}
	bodyHash := sha256.Sum256(payload.RawBody)
	bodyHashHex := hex.EncodeToString(bodyHash[:])
	if subtle.ConstantTimeCompare([]byte(bodySHA256Claim), []byte(bodyHashHex)) != 1 {
		return provider.WebhookEvent{}, fmt.Errorf("body hash mismatch")
	}

	// Step 9: Parse body and map webhook type/code to event.
	return p.parseWebhookBody(payload.RawBody)
}

// getPublicKey retrieves the ECDSA public key for the given kid, using cache.
func (p *PlaidProvider) getPublicKey(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	if cached, ok := p.jwkCache.Load(kid); ok {
		return cached.(*ecdsa.PublicKey), nil
	}

	// Fetch from Plaid API.
	req := plaidgo.NewWebhookVerificationKeyGetRequest(kid)
	resp, _, err := p.client.PlaidApi.WebhookVerificationKeyGet(ctx).WebhookVerificationKeyGetRequest(*req).Execute()
	if err != nil {
		return nil, fmt.Errorf("fetch webhook verification key: %w", err)
	}

	jwk := resp.GetKey()

	// Convert JWK to *ecdsa.PublicKey.
	pubKey, err := jwkToECDSAPublicKey(jwk)
	if err != nil {
		return nil, err
	}

	p.jwkCache.Store(kid, pubKey)
	return pubKey, nil
}

// jwkToECDSAPublicKey converts a Plaid JWK response to an ECDSA P-256 public key.
func jwkToECDSAPublicKey(jwk plaidgo.JWKPublicKey) (*ecdsa.PublicKey, error) {
	xBytes, err := base64.RawURLEncoding.DecodeString(jwk.GetX())
	if err != nil {
		return nil, fmt.Errorf("decode JWK x: %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(jwk.GetY())
	if err != nil {
		return nil, fmt.Errorf("decode JWK y: %w", err)
	}

	curve := elliptic.P256()
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)

	if !curve.IsOnCurve(x, y) {
		return nil, fmt.Errorf("JWK point is not on P-256 curve")
	}

	return &ecdsa.PublicKey{
		Curve: curve,
		X:     x,
		Y:     y,
	}, nil
}

// webhookBody represents the JSON structure of a Plaid webhook body.
type webhookBody struct {
	WebhookType           string          `json:"webhook_type"`
	WebhookCode           string          `json:"webhook_code"`
	ItemID                string          `json:"item_id"`
	Error                 *webhookError   `json:"error"`
	ConsentExpirationTime *string         `json:"consent_expiration_time"`
	NewTransactions       json.RawMessage `json:"new_transactions"`
}

type webhookError struct {
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

// parseWebhookBody maps Plaid webhook_type/webhook_code to a WebhookEvent.
func (p *PlaidProvider) parseWebhookBody(body []byte) (provider.WebhookEvent, error) {
	var wb webhookBody
	if err := json.Unmarshal(body, &wb); err != nil {
		return provider.WebhookEvent{}, fmt.Errorf("parse webhook body: %w", err)
	}

	event := provider.WebhookEvent{
		ConnectionID: wb.ItemID,
	}

	switch {
	case wb.WebhookType == "TRANSACTIONS" && wb.WebhookCode == "SYNC_UPDATES_AVAILABLE":
		event.Type = "sync_available"

	case wb.WebhookType == "ITEM" && wb.WebhookCode == "ERROR":
		event.Type = "connection_error"
		if wb.Error != nil {
			event.ErrorCode = &wb.Error.ErrorCode
			event.ErrorMessage = &wb.Error.ErrorMessage
		}

	case wb.WebhookType == "ITEM" && wb.WebhookCode == "PENDING_EXPIRATION":
		event.Type = "pending_expiration"
		event.ConsentExpirationTime = wb.ConsentExpirationTime

	case wb.WebhookType == "ITEM" && wb.WebhookCode == "NEW_ACCOUNTS_AVAILABLE":
		event.Type = "new_accounts"

	default:
		event.Type = "unknown"
		p.logger.Info("unhandled webhook",
			"webhook_type", wb.WebhookType,
			"webhook_code", wb.WebhookCode,
			"item_id", wb.ItemID,
		)
	}

	return event, nil
}
