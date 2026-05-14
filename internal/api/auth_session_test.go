//go:build !lite

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoleToScope(t *testing.T) {
	cases := []struct {
		role string
		want string
	}{
		{"admin", "full_access"},
		{"editor", "full_access"},
		{"viewer", "read_only"},
		{"", "read_only"},        // unknown / legacy → least privilege
		{"something", "read_only"},
	}
	for _, c := range cases {
		if got := roleToScope(c.role); got != c.want {
			t.Errorf("roleToScope(%q) = %q, want %q", c.role, got, c.want)
		}
	}
}

func TestIsUnsafeMethod(t *testing.T) {
	safe := []string{http.MethodGet, http.MethodHead, http.MethodOptions}
	unsafe := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, m := range safe {
		if isUnsafeMethod(m) {
			t.Errorf("isUnsafeMethod(%q) = true, want false", m)
		}
	}
	for _, m := range unsafe {
		if !isUnsafeMethod(m) {
			t.Errorf("isUnsafeMethod(%q) = false, want true", m)
		}
	}
}

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		origin  string
		referer string
		want    bool
	}{
		{"matching origin", "breadbox.example.com", "https://breadbox.example.com", "", true},
		{"mismatched origin", "breadbox.example.com", "https://evil.com", "", false},
		{"referer fallback matches", "breadbox.example.com", "", "https://breadbox.example.com/v2/transactions", true},
		{"referer fallback mismatched", "breadbox.example.com", "", "https://evil.com/x", false},
		{"no origin, no referer", "breadbox.example.com", "", "", false},
		{"origin takes precedence over referer", "breadbox.example.com", "https://evil.com", "https://breadbox.example.com/x", false},
		{"port-sensitive match", "localhost:8081", "http://localhost:8081", "", true},
		{"port-sensitive mismatch", "localhost:8081", "http://localhost:9090", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://"+c.host+"/api/v1/rules", nil)
			r.Host = c.host
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			if c.referer != "" {
				r.Header.Set("Referer", c.referer)
			}
			if got := sameOrigin(r); got != c.want {
				t.Errorf("sameOrigin() = %v, want %v", got, c.want)
			}
		})
	}
}
