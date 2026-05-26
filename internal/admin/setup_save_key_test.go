//go:build !headless && !lite

package admin

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodeOnePasswordLogin(t *testing.T) {
	const key = "9f3a7d8c2e4b1a6f8c5d3e7b9a1c5d8e3f7b9a2c5d8e1f4b7a9c2e5d8f1b4a7c"
	const title = "Breadbox encryption key (breadbox.local)"

	encoded := encodeOnePasswordLogin(title, key)
	if encoded == "" {
		t.Fatal("encodeOnePasswordLogin returned empty string")
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("output is not valid base64: %v", err)
	}

	var payload struct {
		Title  string `json:"title"`
		Fields []struct {
			Autocomplete string `json:"autocomplete"`
			Value        string `json:"value"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decoded payload is not valid JSON: %v\nbody: %s", err, raw)
	}

	if payload.Title != title {
		t.Errorf("title: got %q, want %q", payload.Title, title)
	}
	if len(payload.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(payload.Fields))
	}
	if payload.Fields[0].Autocomplete != "username" || payload.Fields[0].Value != "ENCRYPTION_KEY" {
		t.Errorf("username field: got %+v", payload.Fields[0])
	}
	if payload.Fields[1].Autocomplete != "current-password" || payload.Fields[1].Value != key {
		t.Errorf("password field: got %+v", payload.Fields[1])
	}
}

func TestSaveKeyItemTitle(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"", "Breadbox encryption key"},
		{"breadbox.local", "Breadbox encryption key (breadbox.local)"},
		{"breadbox.exe.xyz", "Breadbox encryption key (breadbox.exe.xyz)"},
		{"localhost:8080", "Breadbox encryption key (localhost:8080)"},
	}
	for _, tc := range cases {
		got := saveKeyItemTitle(tc.host)
		if got != tc.want {
			t.Errorf("saveKeyItemTitle(%q) = %q, want %q", tc.host, got, tc.want)
		}
		if tc.host != "" && !strings.Contains(got, tc.host) {
			t.Errorf("title for %q does not contain the host: %q", tc.host, got)
		}
	}
}
