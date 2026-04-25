package timefmt

import (
	"testing"
	"time"
)

func TestRelative(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"just now (zero)", now, "just now"},
		{"just now (30s)", now.Add(-30 * time.Second), "just now"},
		{"1 minute", now.Add(-1 * time.Minute), "1 minute ago"},
		{"5 minutes", now.Add(-5 * time.Minute), "5 minutes ago"},
		{"59 minutes", now.Add(-59 * time.Minute), "59 minutes ago"},
		{"1 hour", now.Add(-1 * time.Hour), "1 hour ago"},
		{"3 hours", now.Add(-3 * time.Hour), "3 hours ago"},
		{"23 hours", now.Add(-23 * time.Hour), "23 hours ago"},
		{"1 day", now.Add(-24 * time.Hour), "1 day ago"},
		{"5 days", now.Add(-5 * 24 * time.Hour), "5 days ago"},
		{"future collapses to just now", now.Add(1 * time.Hour), "just now"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Relative(tc.in); got != tc.want {
				t.Errorf("Relative(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
