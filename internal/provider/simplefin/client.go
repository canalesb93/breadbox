//go:build !lite

package simplefin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"breadbox/internal/provider"
)

// Client is a thin HTTP client for the SimpleFIN protocol. It carries no
// credentials of its own — auth is per-connection (the HTTP Basic user:pass is
// embedded in each connection's decrypted access URL).
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new SimpleFIN HTTP client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// claim exchanges a one-time claim URL (the base64-decoded setup token) for a
// long-lived access URL. Per the protocol, this is an unauthenticated POST with
// an empty body; a 200 returns the access URL as the plain-text response body,
// and a 403 means the token was invalid or already claimed.
func (c *Client) claim(ctx context.Context, claimURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, claimURL, nil)
	if err != nil {
		return "", fmt.Errorf("simplefin: build claim request: %w", err)
	}
	req.Header.Set("Content-Length", "0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("simplefin: claim request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden {
			return "", fmt.Errorf("simplefin: claim rejected (403) — setup token is invalid or already used")
		}
		return "", fmt.Errorf("simplefin: claim failed (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	accessURL := strings.TrimSpace(string(body))
	if accessURL == "" {
		return "", fmt.Errorf("simplefin: claim returned an empty access URL")
	}
	if _, err := splitAccessURL(accessURL); err != nil {
		return "", fmt.Errorf("simplefin: claim returned a malformed access URL: %w", err)
	}
	return accessURL, nil
}

// getAccounts calls GET {accessURL}/accounts with the given raw query string
// (no leading "?"), authenticating with the credentials embedded in accessURL.
// Rate-limited (429) responses are retried with backoff.
func (c *Client) getAccounts(ctx context.Context, accessURL, rawQuery string) (*http.Response, error) {
	base, creds, err := splitAccessURLWithCreds(accessURL)
	if err != nil {
		return nil, err
	}

	reqURL := base + "/accounts"
	if rawQuery != "" {
		reqURL += "?" + rawQuery
	}

	var resp *http.Response
	retryErr := provider.DoWithRetry(ctx, provider.DefaultRetryConfig(), func() (bool, error) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if reqErr != nil {
			return false, fmt.Errorf("simplefin: build accounts request: %w", reqErr)
		}
		if creds.user != "" || creds.pass != "" {
			req.SetBasicAuth(creds.user, creds.pass)
		}

		var doErr error
		resp, doErr = c.httpClient.Do(req)
		if doErr != nil {
			return false, fmt.Errorf("simplefin: accounts request: %w", doErr)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			return true, fmt.Errorf("simplefin: rate limited (429)")
		}
		return false, nil
	})
	if retryErr != nil {
		return nil, retryErr
	}
	return resp, nil
}

type basicCreds struct {
	user string
	pass string
}

// splitAccessURL validates an access URL and returns it with userinfo stripped.
func splitAccessURL(accessURL string) (string, error) {
	base, _, err := splitAccessURLWithCreds(accessURL)
	return base, err
}

// splitAccessURLWithCreds parses an access URL of the form
// https://user:pass@host/path and returns the base URL (userinfo removed) and
// the embedded HTTP Basic credentials.
func splitAccessURLWithCreds(accessURL string) (string, basicCreds, error) {
	u, err := url.Parse(strings.TrimSpace(accessURL))
	if err != nil {
		return "", basicCreds{}, fmt.Errorf("parse access URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", basicCreds{}, fmt.Errorf("access URL missing scheme or host")
	}

	var creds basicCreds
	if u.User != nil {
		creds.user = u.User.Username()
		creds.pass, _ = u.User.Password()
	}

	// Rebuild the base URL without the userinfo so credentials never leak into
	// logs or the request line; they travel only in the Authorization header.
	u.User = nil
	base := strings.TrimRight(u.String(), "/")
	return base, creds, nil
}
