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
	if n, u, tr, err := validateConnectorFields("gmail", "https://x/mcp", ""); err != nil || n != "gmail" || u != "https://x/mcp" || tr != "http" {
		t.Fatalf("valid connector: n=%q u=%q tr=%q err=%v", n, u, tr, err)
	}
	cases := map[string][3]string{
		"bad name":      {"Gmail!", "https://x/mcp", "http"},
		"reserved name": {"breadbox", "https://x/mcp", "http"},
		"bad scheme":    {"gmail", "ftp://nope", "http"},
		"no host":       {"gmail", "https://", "http"},
		"bad transport": {"gmail", "https://x/mcp", "stdio"},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := validateConnectorFields(in[0], in[1], in[2]); !errors.Is(err, ErrInvalidParameter) {
				t.Fatalf("want ErrInvalidParameter, got %v", err)
			}
		})
	}
}

func TestHeaderValuesEncryptRoundTrip(t *testing.T) {
	if got, err := encryptHeaderValues(nil, testEncKey); err != nil || got != "" {
		t.Fatalf("empty map: got %q err %v", got, err)
	}
	if _, err := encryptHeaderValues(map[string]string{"Authorization": "x"}, nil); !errors.Is(err, ErrInvalidParameter) {
		t.Fatalf("want ErrInvalidParameter with no key, got %v", err)
	}
	in := map[string]string{"Authorization": "Bearer xyz", "X-Api-Key": "abc"}
	ct, err := encryptHeaderValues(in, testEncKey)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ct == "" || strings.Contains(ct, "Bearer xyz") {
		t.Fatalf("ciphertext leaks plaintext or empty: %q", ct)
	}
	out, err := decryptHeaderValues(ct, testEncKey)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if out["Authorization"] != "Bearer xyz" || out["X-Api-Key"] != "abc" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestNormalizeHeaders(t *testing.T) {
	// Drops blank-name rows; carries forward blank values from existing.
	existing := map[string]string{"Authorization": "Bearer old"}
	in := []ConnectorHeaderInput{
		{Name: "Authorization", Value: ""},      // carry forward
		{Name: "X-Api-Key", Value: "new"},       // new value
		{Name: "  ", Value: "ignored"},          // dropped (blank name)
	}
	names, values, err := normalizeHeaders(in, existing)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(names) != 2 || names[0] != "Authorization" || names[1] != "X-Api-Key" {
		t.Fatalf("names: %v", names)
	}
	if values["Authorization"] != "Bearer old" || values["X-Api-Key"] != "new" {
		t.Fatalf("values: %+v", values)
	}
	// Invalid header name rejected.
	if _, _, err := normalizeHeaders([]ConnectorHeaderInput{{Name: "Bad Header", Value: "x"}}, nil); !errors.Is(err, ErrInvalidParameter) {
		t.Fatalf("want ErrInvalidParameter for bad header name, got %v", err)
	}
	// Duplicate header name rejected.
	if _, _, err := normalizeHeaders([]ConnectorHeaderInput{{Name: "A", Value: "1"}, {Name: "A", Value: "2"}}, nil); !errors.Is(err, ErrInvalidParameter) {
		t.Fatalf("want ErrInvalidParameter for duplicate header, got %v", err)
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
	empty, _ := enabledConnectorNamesToBytes(nil)
	if string(empty) != "[]" {
		t.Fatalf("empty names should encode []: %s", empty)
	}
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
