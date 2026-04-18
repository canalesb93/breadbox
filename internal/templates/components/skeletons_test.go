package components

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestSkeletonComponentsRender asserts every skeleton component produces
// non-empty HTML containing the `bb-skeleton` class. Acts as a smoke test for
// the #462 templ migration — catches regressions where a templ file fails to
// regenerate or the bridge drops a component.
func TestSkeletonComponentsRender(t *testing.T) {
	cases := []struct {
		name string
		fn   func() templ.Component
	}{
		{"SkeletonStatCards", SkeletonStatCards},
		{"SkeletonAccountGrid", SkeletonAccountGrid},
		{"SkeletonTableRows", SkeletonTableRows},
		{"SkeletonTransactionList", SkeletonTransactionList},
		{"SkeletonSyncList", SkeletonSyncList},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			if err := tc.fn().Render(context.Background(), &buf); err != nil {
				t.Fatalf("render: %v", err)
			}
			out := buf.String()
			if !strings.Contains(out, "bb-skeleton") {
				t.Fatalf("expected bb-skeleton class in output; got %q", out)
			}
		})
	}
}
