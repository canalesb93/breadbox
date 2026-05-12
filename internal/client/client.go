// Package client is a thin, hand-written REST client over the
// Breadbox `/api/v1` surface. It covers only the operations the Stage 1
// CLI needs (auth/whoami, version probe, doctor's headless bootstrap);
// later stages will extend it noun-by-noun as the corresponding CLI
// verbs land.
//
// The brief originally called for oapi-codegen output, but the project's
// openapi.yaml has no operationIds — generating a usable typed client
// would require either annotating every operation or generating one
// blob of opaque methods. A hand-written wrapper is smaller, easier to
// review, and fully sufficient for Stage 1.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"breadbox/internal/cli/config"
)

// DefaultTimeout is the per-request timeout applied to every call. Set on
// the http.Client; individual callers can supply a context with a shorter
// deadline.
const DefaultTimeout = 30 * time.Second

// Client wraps an http.Client and knows how to talk to a single configured
// host. Construct via New().
type Client struct {
	host       config.Host
	httpClient *http.Client
	userAgent  string
}

// New builds a Client for the given host. version is the running CLI
// version, surfaced as the User-Agent so server logs can identify the
// caller.
func New(host config.Host, version string) *Client {
	transport := http.DefaultTransport
	// Unix-socket transport — when host.Socket is set, route every request
	// through the named socket. Useful for same-host CLI use where the
	// breadbox server binds a socket alongside the TCP listener.
	if host.Socket != "" {
		t := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", host.Socket)
			},
		}
		transport = t
	}
	return &Client{
		host: host,
		httpClient: &http.Client{
			Timeout:   DefaultTimeout,
			Transport: transport,
		},
		userAgent: fmt.Sprintf("breadbox-cli/%s", version),
	}
}

// BaseURL returns the effective base URL the client is talking to. Useful
// for error messages and `auth status` output.
func (c *Client) BaseURL() string {
	return c.host.BaseURL
}

// APIError is returned for every non-2xx response. It carries the parsed
// `{ "error": { code, message } }` envelope plus the HTTP status so
// callers can map both into CLI exit codes.
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

func (e *APIError) Error() string {
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("%s: %s (HTTP %d)", e.Code, e.Message, e.Status)
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

// errorEnvelope is the on-the-wire shape of error responses; we unwrap
// the `error` key inside Do().
type errorEnvelope struct {
	Error APIError `json:"error"`
}

// Do executes an HTTP request against the configured host and either
// decodes the response into `out` or returns an *APIError. `out` may be
// nil for endpoints that return no body (e.g. 204 responses).
func (c *Client) Do(ctx context.Context, method, path string, body any, out any) error {
	u, err := c.url(path)
	if err != nil {
		return err
	}
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		bodyReader = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if c.host.Token != "" {
		req.Header.Set("X-API-Key", c.host.Token)
	}
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// Try to decode the canonical envelope. If decoding fails (e.g.
		// proxy returned HTML), surface a generic APIError with the
		// status code so the CLI can still map it to an exit code.
		var env errorEnvelope
		raw, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(raw, &env)
		apiErr := env.Error
		apiErr.Status = resp.StatusCode
		if apiErr.Message == "" && len(raw) > 0 {
			apiErr.Message = strings.TrimSpace(string(raw))
		}
		return &apiErr
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		// EOF on an explicitly empty body is fine; surface anything else.
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// url builds the absolute URL for an API path. `path` must start with
// `/api/v1/...` (or another root-level path like `/health`).
func (c *Client) url(path string) (string, error) {
	if c.host.BaseURL == "" {
		return "", errors.New("host base URL is not set")
	}
	base, err := url.Parse(c.host.BaseURL)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}
	rel, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("parse path: %w", err)
	}
	return base.ResolveReference(rel).String(), nil
}

// ----- Stage 1 endpoints -----
// The CLI calls a small fixed set of endpoints. Each one gets a typed
// wrapper here so subcommands stay focused on user output and don't repeat
// JSON struct definitions.

// VersionResponse mirrors the server's GET /api/v1/version payload.
type VersionResponse struct {
	Version         string `json:"version"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable *bool  `json:"update_available"`
	LatestURL       string `json:"latest_url,omitempty"`
}

// Version probes the server's version endpoint. Doubles as a connectivity
// check for `auth login --token` validation.
func (c *Client) Version(ctx context.Context) (*VersionResponse, error) {
	var v VersionResponse
	if err := c.Do(ctx, http.MethodGet, "/api/v1/version", nil, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// WhoamiResponse mirrors GET /api/v1/keys/me.
type WhoamiResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	KeyPrefix string  `json:"key_prefix"`
	Scope     string  `json:"scope"`
	ActorType string  `json:"actor_type"`
	ActorName *string `json:"actor_name,omitempty"`
	CreatedAt string  `json:"created_at"`
}

// Whoami returns the API-key identity for the configured token.
func (c *Client) Whoami(ctx context.Context) (*WhoamiResponse, error) {
	var w WhoamiResponse
	if err := c.Do(ctx, http.MethodGet, "/api/v1/keys/me", nil, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

// HeadlessBootstrap mirrors GET /api/v1/headless/bootstrap and returns the
// raw map so `breadbox doctor` can render every field without the client
// package having to track schema changes one-for-one.
func (c *Client) HeadlessBootstrap(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	if err := c.Do(ctx, http.MethodGet, "/api/v1/headless/bootstrap", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeviceCodeInitResponse mirrors POST /api/v1/auth/device-code. Interval
// and ExpiresIn are in seconds.
type DeviceCodeInitResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// DeviceCodePollResponse mirrors the 200 body of POST
// /api/v1/auth/device-code/poll. Non-2xx responses are surfaced via the
// canonical *APIError envelope (EXPIRED / DENIED / INVALID_DEVICE_CODE).
type DeviceCodePollResponse struct {
	Status string `json:"status"`
	Token  string `json:"token,omitempty"`
}

// InitiateDeviceCode calls POST /api/v1/auth/device-code without auth
// and returns the freshly-minted device + user codes. The caller polls
// PollDeviceCode on the cadence reported in Interval.
func (c *Client) InitiateDeviceCode(ctx context.Context) (*DeviceCodeInitResponse, error) {
	var out DeviceCodeInitResponse
	if err := c.Do(ctx, http.MethodPost, "/api/v1/auth/device-code", struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PollDeviceCode calls POST /api/v1/auth/device-code/poll. Surfaces the
// canonical error envelope via *APIError so callers can branch on
// EXPIRED / DENIED / INVALID_DEVICE_CODE codes without parsing strings.
func (c *Client) PollDeviceCode(ctx context.Context, deviceCode string) (*DeviceCodePollResponse, error) {
	body := struct {
		DeviceCode string `json:"device_code"`
	}{DeviceCode: deviceCode}
	var out DeviceCodePollResponse
	if err := c.Do(ctx, http.MethodPost, "/api/v1/auth/device-code/poll", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
