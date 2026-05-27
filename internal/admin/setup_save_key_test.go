//go:build !headless && !lite

package admin

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// tlsStub stands in for a populated *tls.ConnectionState — its
// presence is all installURL checks (non-nil means TLS terminated at
// us). Field contents don't matter for the test.
var tlsStub = tls.ConnectionState{}

func TestEncodeOnePasswordAPIKey(t *testing.T) {
	const key = "9f3a7d8c2e4b1a6f8c5d3e7b9a1c5d8e3f7b9a2c5d8e1f4b7a9c2e5d8f1b4a7c"
	const title = "Breadbox encryption key (breadbox.local)"
	const url = "https://breadbox.local"

	encoded := encodeOnePasswordAPIKey(title, key, url)
	if encoded == "" {
		t.Fatal("encodeOnePasswordAPIKey returned empty string")
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
	// API Credential items don't take a username; we ship just the
	// secret and the install URL.
	if len(payload.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(payload.Fields))
	}
	if payload.Fields[0].Autocomplete != "current-password" || payload.Fields[0].Value != key {
		t.Errorf("credential field: got %+v", payload.Fields[0])
	}
	if payload.Fields[1].Autocomplete != "url" || payload.Fields[1].Value != url {
		t.Errorf("url field: got %+v", payload.Fields[1])
	}
	// And explicitly: no field should carry the literal "ENCRYPTION_KEY"
	// (the regression this rename was introduced to prevent).
	for _, f := range payload.Fields {
		if f.Value == "ENCRYPTION_KEY" {
			t.Errorf("payload still ships the literal \"ENCRYPTION_KEY\" value: %+v", f)
		}
	}
}

func TestEncodeOnePasswordAPIKeyOmitsEmptyURL(t *testing.T) {
	const key = "9f3a7d8c"
	const title = "Breadbox encryption key"

	encoded := encodeOnePasswordAPIKey(title, key, "")
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	var payload struct {
		Fields []struct {
			Autocomplete string `json:"autocomplete"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(payload.Fields) != 1 {
		t.Fatalf("expected 1 field (credential only) when url is empty, got %d", len(payload.Fields))
	}
	if payload.Fields[0].Autocomplete != "current-password" {
		t.Errorf("expected sole field to be the credential, got %q", payload.Fields[0].Autocomplete)
	}
}

func TestInstallURL(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(*http.Request)
		host   string
		want   string
	}{
		{
			name: "plain HTTP",
			host: "breadbox.local:8080",
			want: "http://breadbox.local:8080",
		},
		{
			name: "TLS sets scheme to https",
			setup: func(r *http.Request) {
				r.TLS = &tlsStub
			},
			host: "breadbox.exe.xyz",
			want: "https://breadbox.exe.xyz",
		},
		{
			name: "X-Forwarded-Proto wins over r.TLS",
			setup: func(r *http.Request) {
				r.Header.Set("X-Forwarded-Proto", "https")
			},
			host: "breadbox.exe.xyz",
			want: "https://breadbox.exe.xyz",
		},
		{
			name: "X-Forwarded-Host wins over r.Host",
			setup: func(r *http.Request) {
				r.Header.Set("X-Forwarded-Host", "public.example.com")
				r.Header.Set("X-Forwarded-Proto", "https")
			},
			host: "internal:8080",
			want: "https://public.example.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/setup/save-key", nil)
			r.Host = tc.host
			if tc.setup != nil {
				tc.setup(r)
			}
			if got := installURL(r); got != tc.want {
				t.Errorf("installURL: got %q, want %q", got, tc.want)
			}
		})
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
