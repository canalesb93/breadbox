//go:build !lite

package teller

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"breadbox/internal/provider"
)

// signTeller builds a valid Teller-Signature header for the given secret/body.
func signTeller(secret string, body []byte, ts int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", ts, body)))
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

// TestVerifySignature_EmptySecretFailsClosed pins the security invariant: when
// no webhook secret is configured the provider must reject every webhook, even
// one carrying a signature an attacker forged with the (publicly known) empty
// HMAC key. Otherwise unverified payloads slip through.
func TestVerifySignature_EmptySecretFailsClosed(t *testing.T) {
	p := &TellerProvider{webhookSecret: ""}

	body := []byte(`{"type":"transactions.processed"}`)
	ts := time.Now().Unix()
	// An attacker can compute this because the empty-key HMAC is deterministic.
	forged := signTeller("", body, ts)

	if err := p.verifySignature(forged, body); err == nil {
		t.Fatal("verifySignature accepted a forged signature with no configured secret; want rejection")
	}
}

// TestVerifySignature_ValidSignatureAccepted confirms the fail-closed guard
// doesn't break the happy path with a real secret.
func TestVerifySignature_ValidSignatureAccepted(t *testing.T) {
	const secret = "whsec_test_secret"
	p := &TellerProvider{webhookSecret: secret}

	body := []byte(`{"type":"transactions.processed"}`)
	ts := time.Now().Unix()
	header := signTeller(secret, body, ts)

	if err := p.verifySignature(header, body); err != nil {
		t.Fatalf("verifySignature rejected a valid signature: %v", err)
	}
}

// TestVerifySignature_WrongSecretRejected confirms a signature minted with a
// different secret is still rejected (constant-time mismatch path).
func TestVerifySignature_WrongSecretRejected(t *testing.T) {
	p := &TellerProvider{webhookSecret: "right_secret"}

	body := []byte(`{"type":"transactions.processed"}`)
	ts := time.Now().Unix()
	header := signTeller("wrong_secret", body, ts)

	if err := p.verifySignature(header, body); err == nil {
		t.Fatal("verifySignature accepted a signature minted with the wrong secret")
	}
}

// TestVerifySignature_StaleTimestampRejected pins the replay-protection window.
func TestVerifySignature_StaleTimestampRejected(t *testing.T) {
	const secret = "whsec_test_secret"
	p := &TellerProvider{webhookSecret: secret}

	body := []byte(`{"type":"transactions.processed"}`)
	ts := time.Now().Add(-10 * time.Minute).Unix()
	header := signTeller(secret, body, ts)

	err := p.verifySignature(header, body)
	if err == nil || !strings.Contains(err.Error(), "too old") {
		t.Fatalf("verifySignature accepted a stale (replayed) webhook: err=%v", err)
	}
}

// TestHandleWebhook_EmptySecretRejected exercises the full entry point to make
// sure the guard sits on the real code path, not just the helper.
func TestHandleWebhook_EmptySecretRejected(t *testing.T) {
	p := &TellerProvider{webhookSecret: ""}

	body := []byte(`{"type":"transactions.processed"}`)
	forged := signTeller("", body, time.Now().Unix())

	_, err := p.HandleWebhook(context.Background(), provider.WebhookPayload{
		RawBody: body,
		Headers: map[string]string{"Teller-Signature": forged},
	})
	if err == nil {
		t.Fatal("HandleWebhook accepted a forged webhook with no configured secret")
	}
}
