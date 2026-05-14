//go:build !lite

package api

import "testing"

func TestRoleToScope(t *testing.T) {
	cases := []struct {
		role string
		want string
	}{
		{"admin", "full_access"},
		{"editor", "full_access"},
		{"viewer", "read_only"},
		{"", "read_only"}, // unknown / legacy → least privilege
		{"something", "read_only"},
	}
	for _, c := range cases {
		if got := roleToScope(c.role); got != c.want {
			t.Errorf("roleToScope(%q) = %q, want %q", c.role, got, c.want)
		}
	}
}
