//go:build !lite

package service

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

// TestVerifyCodeChallenge pins the PKCE verification used by the OAuth
// authorization-code exchange (verifyCodeChallenge → ExchangeAuthorizationCode).
// It's a pure function with no DB dependency, and a regression here is a real
// security hole — accepting a downgraded method or a mis-encoded challenge
// defeats the protection PKCE provides against authorization-code interception.
func TestVerifyCodeChallenge(t *testing.T) {
	// RFC 7636 Appendix B reference vector pins the exact S256 transform:
	// base64url(SHA256(verifier)) with no padding.
	const (
		rfcVerifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		rfcChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	)

	t.Run("rfc7636 vector matches", func(t *testing.T) {
		if !verifyCodeChallenge(rfcVerifier, rfcChallenge, "S256") {
			t.Error("RFC 7636 S256 reference vector should verify")
		}
	})

	t.Run("wrong verifier rejected", func(t *testing.T) {
		if verifyCodeChallenge("not-the-verifier", rfcChallenge, "S256") {
			t.Error("mismatched verifier must not verify")
		}
	})

	// PKCE downgrade protection: only S256 is accepted. "plain" (where the
	// challenge equals the verifier verbatim) and any other/empty method must
	// fail closed — even when the inputs would "match" under plain rules.
	t.Run("plain method rejected", func(t *testing.T) {
		v := "some-verifier-value"
		if verifyCodeChallenge(v, v, "plain") {
			t.Error("plain method must be rejected (server is S256-only)")
		}
	})
	t.Run("empty method rejected", func(t *testing.T) {
		if verifyCodeChallenge(rfcVerifier, rfcChallenge, "") {
			t.Error("empty method must be rejected")
		}
	})
	t.Run("method comparison is case-sensitive", func(t *testing.T) {
		if verifyCodeChallenge(rfcVerifier, rfcChallenge, "s256") {
			t.Error(`only the exact "S256" token is valid per RFC 7636`)
		}
	})
	t.Run("empty challenge rejected", func(t *testing.T) {
		if verifyCodeChallenge(rfcVerifier, "", "S256") {
			t.Error("empty stored challenge must not verify")
		}
	})

	// Encoding guard: a standard-padded base64url encoding of the same digest
	// must NOT verify. This pins the RawURLEncoding choice — a regression to a
	// padded or +/ alphabet encoding would silently break real PKCE clients,
	// and this case catches it.
	t.Run("padded base64url challenge rejected", func(t *testing.T) {
		h := sha256.Sum256([]byte(rfcVerifier))
		padded := base64.URLEncoding.EncodeToString(h[:]) // RawURL form + '=' padding
		if padded == rfcChallenge {
			t.Fatal("test setup: padded form should differ from the raw (unpadded) form")
		}
		if verifyCodeChallenge(rfcVerifier, padded, "S256") {
			t.Error("padded base64url challenge must not verify (RawURLEncoding only)")
		}
	})
}
