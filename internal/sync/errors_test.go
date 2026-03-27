package sync

import "testing"

func TestFriendlyError(t *testing.T) {
	tests := []struct {
		name     string
		rawErr   string
		wantMsg  string
		wantHit  bool // true if we expect a non-empty friendly message
	}{
		// Plaid errors
		{
			name:    "plaid item login required",
			rawErr:  "sync connection abc-123: plaid: item requires re-authentication: provider: re-authentication required",
			wantMsg: "Your bank requires you to re-authenticate.",
			wantHit: true,
		},
		{
			name:    "plaid mutation during pagination",
			rawErr:  "plaid: TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION: transactions mutated",
			wantMsg: "Sync was interrupted by changes at the bank. Will retry automatically.",
			wantHit: true,
		},
		{
			name:    "plaid rate limit",
			rawErr:  "plaid transactions sync: RATE_LIMIT_EXCEEDED",
			wantMsg: "Too many requests to the bank. Will retry later.",
			wantHit: true,
		},
		{
			name:    "plaid institution down",
			rawErr:  "plaid: INSTITUTION_DOWN: the institution is not available",
			wantMsg: "The bank's systems are temporarily unavailable. Will retry later.",
			wantHit: true,
		},
		{
			name:    "plaid institution not responding",
			rawErr:  "plaid: INSTITUTION_NOT_RESPONDING",
			wantMsg: "The bank is not responding. Will retry later.",
			wantHit: true,
		},

		// Teller errors
		{
			name:    "teller enrollment disconnected",
			rawErr:  "sync connection xyz-456: teller: enrollment disconnected: provider: re-authentication required",
			wantMsg: "This bank connection has been disconnected. Please re-authenticate.",
			wantHit: true,
		},
		{
			name:    "teller 403",
			rawErr:  `teller transactions get: status 403: {"error":{"code":"forbidden"}}`,
			wantMsg: "Teller rejected the request. The connection may need re-authentication.",
			wantHit: true,
		},
		{
			name:    "teller 500",
			rawErr:  "teller transactions get: status 500: internal server error",
			wantMsg: "Teller is experiencing issues. Will retry later.",
			wantHit: true,
		},
		{
			name:    "teller fetch accounts",
			rawErr:  "teller: fetch accounts for sync: connection refused",
			wantMsg: "Could not retrieve accounts from Teller.",
			wantHit: true,
		},

		// Generic network errors
		{
			name:    "context deadline exceeded",
			rawErr:  "sync connection abc-123: context deadline exceeded",
			wantMsg: "Sync timed out. The bank may be slow to respond.",
			wantHit: true,
		},
		{
			name:    "context canceled",
			rawErr:  "context canceled",
			wantMsg: "Sync was canceled.",
			wantHit: true,
		},
		{
			name:    "connection refused",
			rawErr:  "Post https://api.teller.io/accounts: dial tcp: connection refused",
			wantMsg: "Could not reach the bank provider. The service may be down.",
			wantHit: true,
		},
		{
			name:    "TLS handshake error",
			rawErr:  "tls: TLS handshake timeout",
			wantMsg: "Secure connection to the bank provider failed.",
			wantHit: true,
		},
		{
			name:    "certificate error",
			rawErr:  "x509: certificate signed by unknown authority",
			wantMsg: "Authentication certificate issue with the bank provider.",
			wantHit: true,
		},
		{
			name:    "i/o timeout",
			rawErr:  "dial tcp 1.2.3.4:443: i/o timeout",
			wantMsg: "Connection to the bank provider timed out.",
			wantHit: true,
		},

		// Internal errors
		{
			name:    "decrypt failure",
			rawErr:  "decrypt access token: cipher: message authentication failed",
			wantMsg: "Could not decrypt connection credentials. Check the encryption key.",
			wantHit: true,
		},
		{
			name:    "server restart",
			rawErr:  "interrupted by server restart",
			wantMsg: "Sync was interrupted by a server restart.",
			wantHit: true,
		},
		{
			name:    "unknown provider",
			rawErr:  "unknown provider: stripe",
			wantMsg: "The provider for this connection is not configured.",
			wantHit: true,
		},

		// Unknown error — should return empty string
		{
			name:    "completely unknown error",
			rawErr:  "something totally unexpected happened in the frobnicator",
			wantMsg: "",
			wantHit: false,
		},
		{
			name:    "empty string",
			rawErr:  "",
			wantMsg: "",
			wantHit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FriendlyError(tt.rawErr)
			if tt.wantHit && got == "" {
				t.Errorf("FriendlyError(%q) returned empty, wanted %q", tt.rawErr, tt.wantMsg)
			}
			if !tt.wantHit && got != "" {
				t.Errorf("FriendlyError(%q) = %q, wanted empty string", tt.rawErr, got)
			}
			if tt.wantHit && got != tt.wantMsg {
				t.Errorf("FriendlyError(%q) = %q, want %q", tt.rawErr, got, tt.wantMsg)
			}
		})
	}
}

func TestFriendlyError_FirstMatchWins(t *testing.T) {
	// "teller: fetch accounts for sync: connection refused" should match the
	// teller-specific pattern first, not the generic "connection refused".
	raw := "teller: fetch accounts for sync: connection refused"
	got := FriendlyError(raw)
	want := "Could not retrieve accounts from Teller."
	if got != want {
		t.Errorf("FriendlyError(%q) = %q, want %q (first match should win)", raw, got, want)
	}
}
