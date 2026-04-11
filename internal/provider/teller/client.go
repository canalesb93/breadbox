package teller

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"breadbox/internal/provider"
)

// Client is an mTLS HTTP client for the Teller API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new Teller API client using the provided certificate
// and private key files for mTLS authentication.
func NewClient(certPath, keyPath string) (*Client, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load teller certificate: %w", err)
	}
	return newClientWithCert(cert), nil
}

// NewClientFromPEM creates a new Teller API client using PEM-encoded
// certificate and private key bytes (e.g. stored encrypted in the database).
func NewClientFromPEM(certPEM, keyPEM []byte) (*Client, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse teller certificate PEM: %w", err)
	}
	return newClientWithCert(cert), nil
}

func newClientWithCert(cert tls.Certificate) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	}
	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		baseURL: "https://api.teller.io",
	}
}

// doWithAuth sends an authenticated HTTP request to the Teller API.
// The access token is sent as the username in HTTP Basic Auth (empty password).
func (c *Client) doWithAuth(ctx context.Context, method, path, accessToken string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth(accessToken, "")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpClient.Do(req)
}

// doWithRetry sends an authenticated request with exponential backoff on 429
// responses. The body is passed as a string so it can be re-read on retry.
func (c *Client) doWithRetry(ctx context.Context, method, path, accessToken, bodyStr string) (*http.Response, error) {
	var resp *http.Response

	err := provider.DoWithRetry(ctx, provider.DefaultRetryConfig(), func() (bool, error) {
		var body io.Reader
		if bodyStr != "" {
			body = strings.NewReader(bodyStr)
		}

		var err error
		resp, err = c.doWithAuth(ctx, method, path, accessToken, body)
		if err != nil {
			return false, err
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			return true, fmt.Errorf("teller rate limited: %d", resp.StatusCode)
		}

		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}
