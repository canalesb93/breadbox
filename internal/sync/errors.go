package sync

import "strings"

// errorMapping maps a substring pattern to a user-friendly error message.
type errorMapping struct {
	pattern string
	message string
}

// knownErrors is an ordered list of error patterns and their friendly messages.
// Patterns are checked in order; the first match wins.
var knownErrors = []errorMapping{
	// Plaid-specific errors
	{pattern: "ITEM_LOGIN_REQUIRED", message: "Your bank requires you to re-authenticate."},
	{pattern: "INVALID_CREDENTIALS", message: "Your bank credentials are incorrect. Please re-authenticate."},
	{pattern: "ITEM_LOCKED", message: "Too many failed login attempts. Your bank account may be locked."},
	{pattern: "INSUFFICIENT_CREDENTIALS", message: "Additional credentials are needed. Please re-authenticate."},
	{pattern: "MFA_NOT_SUPPORTED", message: "Multi-factor authentication is required but not supported for this connection."},
	{pattern: "NO_ACCOUNTS", message: "No accounts were found for this connection."},
	{pattern: "TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION", message: "Sync was interrupted by changes at the bank. Will retry automatically."},
	{pattern: "RATE_LIMIT_EXCEEDED", message: "Too many requests to the bank. Will retry later."},
	{pattern: "INSTITUTION_DOWN", message: "The bank's systems are temporarily unavailable. Will retry later."},
	{pattern: "INSTITUTION_NOT_RESPONDING", message: "The bank is not responding. Will retry later."},
	{pattern: "INSTITUTION_NOT_AVAILABLE", message: "The bank is temporarily unavailable. Will retry later."},
	{pattern: "PLANNED_MAINTENANCE", message: "The bank is undergoing planned maintenance. Will retry later."},
	{pattern: "item requires re-authentication", message: "Your bank requires you to re-authenticate."},
	{pattern: "plaid transactions sync", message: "Failed to sync transactions from Plaid."},
	{pattern: "plaid accounts get", message: "Failed to fetch account data from Plaid."},

	// Teller-specific errors
	{pattern: "enrollment disconnected", message: "This bank connection has been disconnected. Please re-authenticate."},
	{pattern: "enrollment.disconnected", message: "This bank connection has been disconnected. Please re-authenticate."},
	{pattern: "teller transactions get: status 403", message: "Teller rejected the request. The connection may need re-authentication."},
	{pattern: "teller transactions get: status 404", message: "Teller could not find the account. It may have been closed or removed."},
	{pattern: "rate limited (status 429)", message: "Too many requests to Teller. Will retry later."},
	{pattern: "server error (status 5", message: "Teller is experiencing issues. Will retry later."},
	{pattern: "teller: fetch accounts for sync", message: "Could not retrieve accounts from Teller."},
	{pattern: "teller transactions decode", message: "Received unexpected data from Teller."},

	// Provider-agnostic / network errors
	{pattern: "context deadline exceeded", message: "Sync timed out. The bank may be slow to respond."},
	{pattern: "context canceled", message: "Sync was canceled."},
	{pattern: "connection refused", message: "Could not reach the bank provider. The service may be down."},
	{pattern: "connection reset by peer", message: "The connection to the bank provider was interrupted."},
	{pattern: "no such host", message: "Could not resolve the bank provider's address."},
	{pattern: "TLS handshake", message: "Secure connection to the bank provider failed."},
	{pattern: "certificate", message: "Authentication certificate issue with the bank provider."},
	{pattern: "i/o timeout", message: "Connection to the bank provider timed out."},
	{pattern: "EOF", message: "The bank provider closed the connection unexpectedly."},

	// Internal / database errors
	{pattern: "decrypt access token", message: "Could not decrypt connection credentials. Check the encryption key."},
	{pattern: "load connection", message: "Could not load the connection from the database."},
	{pattern: "unknown provider", message: "The provider for this connection is not configured."},
	{pattern: "begin transaction", message: "Database error while starting sync."},
	{pattern: "commit transaction", message: "Database error while saving sync results."},
	{pattern: "interrupted by server restart", message: "Sync was interrupted by a server restart."},
}

// FriendlyError returns a user-friendly error message for the given raw error
// string. If no known pattern matches, it returns an empty string, indicating
// that the raw message should be displayed as-is.
func FriendlyError(rawErr string) string {
	lower := strings.ToLower(rawErr)
	for _, m := range knownErrors {
		if strings.Contains(lower, strings.ToLower(m.pattern)) {
			return m.message
		}
	}
	return ""
}
