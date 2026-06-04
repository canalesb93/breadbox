//go:build !lite

package service

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"breadbox/internal/agent"
	"breadbox/internal/crypto"
)

// Custom MCP connectors (Phase 1) let a workflow reach additional MCP servers
// beyond the built-in breadbox stdio MCP — e.g. a remote Gmail MCP so the
// reviewer agent can look up receipts. Phase 1 is HTTP-transport,
// bring-your-own-credential: the operator supplies a URL + header, and the
// secret header value is stored AES-256-GCM-encrypted in workflows.connectors.
//
// The secret never round-trips in plaintext: handlers encrypt it (they hold the
// ENCRYPTION_KEY) into ConnectorInput.SecretCiphertext; reads return only
// ConnectorView, which reports has_secret but never the value; run assembly
// decrypts straight into the sidecar's per-server Headers map.

// ConnectorInput is the write shape for one connector. Secret is the plaintext
// header value (e.g. "Bearer sk-…"); the service encrypts it with the app's
// ENCRYPTION_KEY before storage. An empty Secret means "keep the existing
// secret" (update, matched by name) or "no secret" (create).
type ConnectorInput struct {
	Name       string
	URL        string
	HeaderName string
	Secret     string
}

// ConnectorView is the read shape returned to clients. It never carries the
// secret (plaintext or ciphertext) — only whether one is set.
type ConnectorView struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	HeaderName string `json:"header_name"`
	HasSecret  bool   `json:"has_secret"`
}

// storedConnector is the on-disk JSONB shape in workflows.connectors.
type storedConnector struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	HeaderName string `json:"header_name,omitempty"`
	Secret     string `json:"secret_ciphertext,omitempty"` // hex(AES-256-GCM)
}

// reservedConnectorName is the always-present breadbox MCP server key; a custom
// connector may not shadow it.
const reservedConnectorName = "breadbox"

// validConnectorName constrains the name to a safe MCP server key — it becomes
// the "mcp__<name>" tool prefix in the SDK allow-list.
var validConnectorName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)

// encryptConnectorSecret encrypts a plaintext connector secret (the full header
// value, e.g. "Bearer sk-…") into hex ciphertext for storage. An empty
// plaintext yields "" (no secret). A non-empty secret with no ENCRYPTION_KEY
// configured is a hard error — we never store a connector secret in the clear.
func encryptConnectorSecret(plaintext string, encKey []byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if len(encKey) == 0 {
		return "", fmt.Errorf("%w: ENCRYPTION_KEY must be set to store a connector secret", ErrInvalidParameter)
	}
	ct, err := crypto.Encrypt([]byte(plaintext), encKey)
	if err != nil {
		return "", fmt.Errorf("encrypt connector secret: %w", err)
	}
	return hex.EncodeToString(ct), nil
}

func storedConnectorsFromBytes(b []byte) []storedConnector {
	if len(b) == 0 {
		return nil
	}
	var out []storedConnector
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

func connectorsToBytes(cs []storedConnector) ([]byte, error) {
	if len(cs) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(cs)
}

// connectorViewsFromBytes decodes the stored JSONB into client-facing views,
// stripping every secret down to a has_secret boolean.
func connectorViewsFromBytes(b []byte) []ConnectorView {
	stored := storedConnectorsFromBytes(b)
	views := make([]ConnectorView, 0, len(stored))
	for _, c := range stored {
		views = append(views, ConnectorView{
			Name:       c.Name,
			URL:        c.URL,
			HeaderName: c.HeaderName,
			HasSecret:  c.Secret != "",
		})
	}
	return views
}

// buildStoredConnectors validates incoming connector inputs, encrypts any newly
// supplied secrets, and merges them with the existing stored set (matched by
// name), carrying the existing ciphertext forward when an input omits its
// secret. Returns JSONB bytes for the column. Rejects bad names, the reserved
// "breadbox" name, duplicates, and non-http(s) URLs. Working from the RAW
// existing bytes (not a masked response) is what keeps an edit that doesn't
// re-enter the secret from wiping it.
func buildStoredConnectors(inputs []ConnectorInput, existing, encKey []byte) ([]byte, error) {
	prev := map[string]string{} // name -> ciphertext
	for _, c := range storedConnectorsFromBytes(existing) {
		prev[c.Name] = c.Secret
	}
	seen := map[string]bool{}
	out := make([]storedConnector, 0, len(inputs))
	for _, in := range inputs {
		name := strings.TrimSpace(in.Name)
		if !validConnectorName.MatchString(name) {
			return nil, fmt.Errorf("%w: connector name %q must match [a-z][a-z0-9_]{0,30}", ErrInvalidParameter, in.Name)
		}
		if name == reservedConnectorName {
			return nil, fmt.Errorf("%w: connector name %q is reserved", ErrInvalidParameter, name)
		}
		if seen[name] {
			return nil, fmt.Errorf("%w: duplicate connector name %q", ErrInvalidParameter, name)
		}
		seen[name] = true

		raw := strings.TrimSpace(in.URL)
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, fmt.Errorf("%w: connector %q url must be a valid http(s) URL", ErrInvalidParameter, name)
		}

		// New secret → encrypt; blank secret → carry forward the stored one.
		secret, err := encryptConnectorSecret(strings.TrimSpace(in.Secret), encKey)
		if err != nil {
			return nil, err
		}
		if secret == "" {
			secret = prev[name]
		}
		header := strings.TrimSpace(in.HeaderName)
		if secret != "" && header == "" {
			return nil, fmt.Errorf("%w: connector %q has a secret but no header name", ErrInvalidParameter, name)
		}
		out = append(out, storedConnector{
			Name:       name,
			URL:        raw,
			HeaderName: header,
			Secret:     secret,
		})
	}
	return connectorsToBytes(out)
}

// connectorServersFromBytes decodes stored connectors and decrypts their
// secrets into agent.MCPServerConfig HTTP entries plus the matching
// "mcp__<name>" allow-list prefixes. Called at run assembly, where encKey is
// available. A connector with no secret mounts with no headers.
func connectorServersFromBytes(b, encKey []byte) (map[string]agent.MCPServerConfig, []string, error) {
	stored := storedConnectorsFromBytes(b)
	servers := make(map[string]agent.MCPServerConfig, len(stored))
	tools := make([]string, 0, len(stored))
	for _, c := range stored {
		cfg := agent.MCPServerConfig{Type: "http", URL: c.URL}
		if c.Secret != "" && c.HeaderName != "" {
			ct, err := hex.DecodeString(c.Secret)
			if err != nil {
				return nil, nil, fmt.Errorf("connector %q: decode secret: %w", c.Name, err)
			}
			plain, err := crypto.Decrypt(ct, encKey)
			if err != nil {
				return nil, nil, fmt.Errorf("connector %q: decrypt secret: %w", c.Name, err)
			}
			cfg.Headers = map[string]string{c.HeaderName: string(plain)}
		}
		servers[c.Name] = cfg
		tools = append(tools, "mcp__"+c.Name)
	}
	return servers, tools, nil
}
