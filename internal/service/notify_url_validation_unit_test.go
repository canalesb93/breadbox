//go:build !lite

package service

import (
	"errors"
	"testing"
)

// T3NotifyURLValid verifies that well-formed http and https URLs with a host
// are accepted by validateNotifyURL.
func TestT3NotifyURLValid(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"http with host", "http://example.com/hook"},
		{"https with host", "https://ntfy.sh/topic"},
		{"http with port", "http://localhost:9000/webhook"},
		{"https with path and query", "https://hooks.example.com/services/x/y/z?token=abc"},
		{"https IP host", "https://192.168.1.1/notify"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNotifyURL(tc.url)
			if err != nil {
				t.Errorf("validateNotifyURL(%q) = %v, want nil", tc.url, err)
			}
		})
	}
}

// T3NotifyURLInvalid verifies that validateNotifyURL returns ErrInvalidParameter
// for empty strings, URLs missing a scheme, ftp:// URLs, and host-less URLs.
func TestT3NotifyURLInvalid(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"empty string", ""},
		{"missing scheme bare host", "example.com/hook"},
		{"missing scheme no slash", "example.com"},
		{"ftp scheme", "ftp://example.com/resource"},
		{"host-less https", "https://"},
		{"host-less http", "http://"},
		{"no scheme path only", "/just/a/path"},
		{"javascript scheme", "javascript:alert(1)"},
		{"file scheme", "file:///etc/passwd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNotifyURL(tc.url)
			if err == nil {
				t.Errorf("validateNotifyURL(%q) = nil, want error", tc.url)
				return
			}
			if !errors.Is(err, ErrInvalidParameter) {
				t.Errorf("validateNotifyURL(%q) error = %v, want errors.Is(err, ErrInvalidParameter) = true", tc.url, err)
			}
		})
	}
}

// T3NotifyURLErrWrapping verifies that the error returned by validateNotifyURL
// wraps ErrInvalidParameter so callers can use errors.Is for dispatch.
func TestT3NotifyURLErrWrapping(t *testing.T) {
	err := validateNotifyURL("ftp://example.com")
	if err == nil {
		t.Fatal("expected error for ftp scheme, got nil")
	}
	if !errors.Is(err, ErrInvalidParameter) {
		t.Errorf("expected error to wrap ErrInvalidParameter, got: %v", err)
	}
}
