//go:build !lite

package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"breadbox/internal/agent"
)

// testEncKey is a deterministic 32-byte AES-256 key for connector crypto tests.
var testEncKey = []byte("0123456789abcdef0123456789abcdef")

func TestValidateConnectorFields(t *testing.T) {
	if _, _, err := validateConnectorFields("gmail", "https://x/mcp"); err != nil {
		t.Fatalf("valid connector rejected: %v", err)
	}
	cases := map[string][2]string{
		"bad name":      {"Gmail!", "https://x/mcp"},
		"reserved name": {"breadbox", "https://x/mcp"},
		"bad scheme":    {"gmail", "ftp://nope"},
		"no host":       {"gmail", "https://"},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := validateConnectorFields(in[0], in[1]); !errors.Is(err, ErrInvalidParameter) {
				t.Fatalf("want ErrInvalidParameter, got %v", err)
			}
		})
	}
}

func TestEncryptConnectorSecret(t *testing.T) {
	if got, err := encryptConnectorSecret("", testEncKey); err != nil || got != "" {
		t.Fatalf("empty plaintext: got %q err %v", got, err)
	}
	if _, err := encryptConnectorSecret("tok", nil); !errors.Is(err, ErrInvalidParameter) {
		t.Fatalf("want ErrInvalidParameter with no key, got %v", err)
	}
	ct, err := encryptConnectorSecret("Bearer xyz", testEncKey)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ct == "" || strings.Contains(ct, "Bearer xyz") {
		t.Fatalf("ciphertext leaks plaintext or is empty: %q", ct)
	}
}

func TestEnabledConnectorNamesRoundTrip(t *testing.T) {
	b, err := enabledConnectorNamesToBytes([]string{"gmail", "gmail", " calendar ", ""})
	if err != nil {
		t.Fatalf("to bytes: %v", err)
	}
	names := enabledConnectorNamesFromBytes(b)
	if len(names) != 2 || names[0] != "gmail" || names[1] != "calendar" {
		t.Fatalf("dedup/trim failed: %v", names)
	}
	// Empty → "[]".
	empty, _ := enabledConnectorNamesToBytes(nil)
	if string(empty) != "[]" {
		t.Fatalf("empty names should encode []: %s", empty)
	}
	// Invalid name rejected.
	if _, err := enabledConnectorNamesToBytes([]string{"Bad Name"}); !errors.Is(err, ErrInvalidParameter) {
		t.Fatalf("want ErrInvalidParameter for bad name, got %v", err)
	}
}

// TestConnectorMCPServerJSON_OmitsCommand guards the TS-union contract: an HTTP
// connector must not serialize an empty "command", or the sidecar's zod union
// would match it against the stdio variant.
func TestConnectorMCPServerJSON_OmitsCommand(t *testing.T) {
	cfg := agent.MCPServerConfig{Type: "http", URL: "https://gmail/mcp", Headers: map[string]string{"Authorization": "Bearer x"}}
	out, _ := json.Marshal(cfg)
	if strings.Contains(string(out), "\"command\"") {
		t.Fatalf("http connector serialized a command key: %s", out)
	}
	if !strings.Contains(string(out), "\"type\":\"http\"") {
		t.Fatalf("http connector missing type=http: %s", out)
	}
}
