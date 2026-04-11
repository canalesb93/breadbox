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

// classifyHTTPError returns an appropriate sentinel-wrapped error for non-OK
// Teller API responses. Server errors (5xx) and rate limits (429) are wrapped
// with ErrSyncRetryable so the sync engine can retry. Other errors are returned
// as-is with context.
func classifyHTTPError(operation string, statusCode int, body []byte) error {
	switch {
	case statusCode == http.StatusTooManyRequests:
		return fmt.Errorf("teller %s: rate limited (status %d): %w", operation, statusCode, provider.ErrSyncRetryable)
	case statusCode >= 500:
		return fmt.Errorf("teller %s: server error (status %d): %s: %w", operation, statusCode, body, provider.ErrSyncRetryable)
	default:
		return fmt.Errorf("teller %s: status %d: %s", operation, statusCode, body)
	}
}
