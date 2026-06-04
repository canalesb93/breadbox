//go:build !lite

package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// testEncKey is a deterministic 32-byte AES-256 key for connector crypto tests.
var testEncKey = []byte("0123456789abcdef0123456789abcdef")

func TestBuildStoredConnectors_EncryptsAndValidates(t *testing.T) {
	in := []ConnectorInput{{
		Name:       "gmail",
		URL:        "https://gmail-mcp.example.com/mcp",
		HeaderName: "Authorization",
		Secret:     "Bearer sk-secret-123",
	}}
	b, err := buildStoredConnectors(in, nil, testEncKey)
	if err != nil {
		t.Fatalf("buildStoredConnectors: %v", err)
	}
	if strings.Contains(string(b), "Bearer sk-secret-123") {
		t.Fatalf("stored connectors contain plaintext secret: %s", b)
	}
	stored := storedConnectorsFromBytes(b)
	if len(stored) != 1 || stored[0].Name != "gmail" || stored[0].URL != "https://gmail-mcp.example.com/mcp" {
		t.Fatalf("unexpected stored connector: %+v", stored)
	}
	if stored[0].Secret == "" {
		t.Fatalf("expected encrypted secret to be stored")
	}
}

func TestBuildStoredConnectors_Rejects(t *testing.T) {
	cases := map[string][]ConnectorInput{
		"bad name":       {{Name: "Gmail!", URL: "https://x/mcp"}},
		"reserved name":  {{Name: "breadbox", URL: "https://x/mcp"}},
		"bad url":        {{Name: "gmail", URL: "ftp://nope"}},
		"secret no head": {{Name: "gmail", URL: "https://x/mcp", Secret: "tok"}},
		"duplicate": {
			{Name: "gmail", URL: "https://x/mcp"},
			{Name: "gmail", URL: "https://y/mcp"},
		},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := buildStoredConnectors(in, nil, testEncKey); !errors.Is(err, ErrInvalidParameter) {
				t.Fatalf("want ErrInvalidParameter, got %v", err)
			}
		})
	}
}

func TestBuildStoredConnectors_CarryForwardAndReplace(t *testing.T) {
	existing, err := buildStoredConnectors([]ConnectorInput{{
		Name: "gmail", URL: "https://x/mcp", HeaderName: "Authorization", Secret: "old-token",
	}}, nil, testEncKey)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	oldCipher := storedConnectorsFromBytes(existing)[0].Secret

	// Blank secret on edit → carry the stored ciphertext forward unchanged.
	kept, err := buildStoredConnectors([]ConnectorInput{{
		Name: "gmail", URL: "https://x/mcp", HeaderName: "Authorization", Secret: "",
	}}, existing, testEncKey)
	if err != nil {
		t.Fatalf("carry-forward: %v", err)
	}
	if got := storedConnectorsFromBytes(kept)[0].Secret; got != oldCipher {
		t.Fatalf("expected secret carried forward, got %q want %q", got, oldCipher)
	}

	// New secret → re-encrypted; ciphertext differs and decrypts to the new value.
	replaced, err := buildStoredConnectors([]ConnectorInput{{
		Name: "gmail", URL: "https://x/mcp", HeaderName: "Authorization", Secret: "new-token",
	}}, existing, testEncKey)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	newCipher := storedConnectorsFromBytes(replaced)[0].Secret
	if newCipher == oldCipher {
		t.Fatalf("expected re-encrypted secret to differ from old ciphertext")
	}
}

func TestBuildStoredConnectors_SecretNeedsEncKey(t *testing.T) {
	_, err := buildStoredConnectors([]ConnectorInput{{
		Name: "gmail", URL: "https://x/mcp", HeaderName: "Authorization", Secret: "tok",
	}}, nil, nil)
	if !errors.Is(err, ErrInvalidParameter) {
		t.Fatalf("want ErrInvalidParameter when encKey missing, got %v", err)
	}
	// A connector with no secret stores fine without a key.
	if _, err := buildStoredConnectors([]ConnectorInput{{
		Name: "gmail", URL: "https://x/mcp",
	}}, nil, nil); err != nil {
		t.Fatalf("no-secret connector should not need encKey: %v", err)
	}
}

func TestConnectorViewsFromBytes_NoSecretLeak(t *testing.T) {
	b, _ := buildStoredConnectors([]ConnectorInput{{
		Name: "gmail", URL: "https://x/mcp", HeaderName: "Authorization", Secret: "tok",
	}}, nil, testEncKey)
	views := connectorViewsFromBytes(b)
	if len(views) != 1 {
		t.Fatalf("want 1 view, got %d", len(views))
	}
	if !views[0].HasSecret {
		t.Fatalf("expected HasSecret=true")
	}
	// The view struct carries only has_secret — confirm neither the ciphertext
	// key nor the secret value escapes. (has_secret legitimately contains the
	// substring "secret", so we ban the value + the ciphertext key, not it.)
	out, _ := json.Marshal(views[0])
	for _, banned := range []string{"ciphertext", "secret_ciphertext", "tok"} {
		if strings.Contains(string(out), banned) {
			t.Fatalf("connector view leaked %q: %s", banned, out)
		}
	}
}

func TestConnectorServersFromBytes_DecryptsToHTTP(t *testing.T) {
	b, _ := buildStoredConnectors([]ConnectorInput{{
		Name: "gmail", URL: "https://gmail/mcp", HeaderName: "Authorization", Secret: "Bearer xyz",
	}}, nil, testEncKey)
	servers, tools, err := connectorServersFromBytes(b, testEncKey)
	if err != nil {
		t.Fatalf("connectorServersFromBytes: %v", err)
	}
	cfg, ok := servers["gmail"]
	if !ok {
		t.Fatalf("missing gmail server")
	}
	if cfg.Type != "http" || cfg.URL != "https://gmail/mcp" {
		t.Fatalf("unexpected server cfg: %+v", cfg)
	}
	if cfg.Headers["Authorization"] != "Bearer xyz" {
		t.Fatalf("header not decrypted: %+v", cfg.Headers)
	}
	if len(tools) != 1 || tools[0] != "mcp__gmail" {
		t.Fatalf("unexpected tools: %v", tools)
	}
}

// TestConnectorMCPServerJSON_OmitsCommand guards the TS-union contract: an HTTP
// connector must not serialize an empty "command", or the sidecar's zod union
// would match it against the stdio variant.
func TestConnectorMCPServerJSON_OmitsCommand(t *testing.T) {
	b, _ := buildStoredConnectors([]ConnectorInput{{
		Name: "gmail", URL: "https://gmail/mcp", HeaderName: "Authorization", Secret: "tok",
	}}, nil, testEncKey)
	servers, _, _ := connectorServersFromBytes(b, testEncKey)
	out, _ := json.Marshal(servers["gmail"])
	if strings.Contains(string(out), "\"command\"") {
		t.Fatalf("http connector serialized a command key: %s", out)
	}
	if !strings.Contains(string(out), "\"type\":\"http\"") {
		t.Fatalf("http connector missing type=http: %s", out)
	}
}
