//go:build !lite

package simplefin

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	"breadbox/internal/provider"
)

// ErrReauthRequired indicates the SimpleFIN access URL has been revoked or
// disabled and the user must paste a fresh setup token.
var ErrReauthRequired = fmt.Errorf("simplefin: access revoked: %w", provider.ErrReauthRequired)

// decodeSetupToken base64-decodes a SimpleFIN setup token into its claim URL.
// SimpleFIN setup tokens are standard-base64-encoded URLs; some servers omit
// padding, so both padded and raw encodings are accepted.
func decodeSetupToken(setupToken string) (string, error) {
	s := strings.TrimSpace(setupToken)
	if s == "" {
		return "", fmt.Errorf("simplefin: empty setup token")
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := enc.DecodeString(s); err == nil {
			claimURL := strings.TrimSpace(string(decoded))
			if strings.HasPrefix(claimURL, "http://") || strings.HasPrefix(claimURL, "https://") {
				return claimURL, nil
			}
		}
	}
	return "", fmt.Errorf("simplefin: setup token is not a base64-encoded claim URL")
}

// fetchAccountSet performs an authenticated GET {accessURL}/accounts with the
// given raw query, maps transport/HTTP failures onto the provider sentinels,
// decodes the response, and logs any soft errors the server reported.
func (p *SimpleFINProvider) fetchAccountSet(ctx context.Context, accessURL, rawQuery string) (accountSet, error) {
	resp, err := p.client.getAccounts(ctx, accessURL, rawQuery)
	if err != nil {
		return accountSet{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return accountSet{}, ErrReauthRequired
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return accountSet{}, classifyHTTPError("accounts get", resp.StatusCode, body)
	}

	set, err := decodeAccountSet(resp.Body)
	if err != nil {
		return accountSet{}, err
	}

	if errs := set.errorStrings(); len(errs) > 0 {
		p.logger.WarnContext(ctx, "simplefin: server reported errors", "errors", errs)
	}

	return set, nil
}

// classifyHTTPError maps a non-OK SimpleFIN response to a descriptive error.
// As with Teller, 429s are already retried at the HTTP layer (DoWithRetry) and
// nothing here is wrapped in ErrSyncRetryable — the engine's cursor-reset retry
// loop is Plaid-specific, so a failed SimpleFIN sync simply retries next tick.
func classifyHTTPError(operation string, statusCode int, body []byte) error {
	trimmed := strings.TrimSpace(string(body))
	switch {
	case statusCode == http.StatusPaymentRequired:
		return fmt.Errorf("simplefin %s: payment required (402) — the SimpleFIN subscription may have lapsed", operation)
	case statusCode == http.StatusTooManyRequests:
		return fmt.Errorf("simplefin %s: rate limited (429) — exceeded the daily request budget", operation)
	case statusCode >= 500:
		return fmt.Errorf("simplefin %s: server error (status %d): %s", operation, statusCode, trimmed)
	default:
		return fmt.Errorf("simplefin %s: status %d: %s", operation, statusCode, trimmed)
	}
}
