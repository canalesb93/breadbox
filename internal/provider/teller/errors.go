package teller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"breadbox/internal/provider"
)

// ErrReauthRequired indicates that the Teller enrollment is disconnected
// and the user needs to re-authenticate.
var ErrReauthRequired = fmt.Errorf("teller: enrollment disconnected: %w", provider.ErrReauthRequired)

// isReauthResponse checks if a Teller API response indicates the enrollment
// is disconnected and requires re-authentication. Teller returns 403 for some
// disconnects and 404 with "enrollment.disconnected.*" codes for others (e.g., MFA required).
// The response body is consumed and the caller should not read it again.
func isReauthResponse(resp *http.Response) bool {
	if resp.StatusCode == http.StatusForbidden {
		return true
	}
	if resp.StatusCode == http.StatusNotFound {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false
		}
		var tellerErr struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &tellerErr) == nil &&
			strings.HasPrefix(tellerErr.Error.Code, "enrollment.disconnected") {
			return true
		}
	}
	return false
}

// classifyHTTPError returns a contextual error for non-OK Teller API responses.
// Reauth cases (403, enrollment.disconnected) are handled separately by
// isReauthResponse. Rate limits and server errors are NOT wrapped with
// ErrSyncRetryable — Teller retry is handled by the DoWithRetry helper at the
// HTTP call site, not by the sync engine's cursor-reset loop (which is designed
// only for Plaid's mutation-during-pagination).
func classifyHTTPError(operation string, statusCode int, body []byte) error {
	switch {
	case statusCode == http.StatusTooManyRequests:
		return fmt.Errorf("teller %s: rate limited (status %d)", operation, statusCode)
	case statusCode >= 500:
		return fmt.Errorf("teller %s: server error (status %d): %s", operation, statusCode, body)
	default:
		return fmt.Errorf("teller %s: status %d: %s", operation, statusCode, body)
	}
}
