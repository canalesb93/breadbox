//go:build !lite

package service

import "testing"

func TestValidateNotifyURL(t *testing.T) {
	ok := []string{"https://ntfy.sh/topic", "http://localhost:8080/hook", "https://hooks.slack.com/services/x/y/z"}
	for _, u := range ok {
		if err := validateNotifyURL(u); err != nil {
			t.Errorf("validateNotifyURL(%q) = %v, want nil", u, err)
		}
	}
	bad := []string{"", "ftp://x", "notaurl", "javascript:alert(1)", "https://"}
	for _, u := range bad {
		if err := validateNotifyURL(u); err == nil {
			t.Errorf("validateNotifyURL(%q) = nil, want error", u)
		}
	}
}
