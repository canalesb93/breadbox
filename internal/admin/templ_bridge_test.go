package admin

import (
	"strings"
	"testing"
)

// TestRenderComponentBridge asserts the templ-bridge funcMap helper renders
// each registered component and returns a safe fallback for unknown names.
// Protects the incremental #462 migration from regressions where a component
// is dropped from the registry or a templ file fails to regenerate.
func TestRenderComponentBridge(t *testing.T) {
	for name := range templComponents {
		t.Run(name, func(t *testing.T) {
			out := string(renderComponent(name))
			if !strings.Contains(out, "bb-skeleton") {
				t.Fatalf("expected bb-skeleton class in rendered %q, got %q", name, out)
			}
		})
	}

	t.Run("unknown name", func(t *testing.T) {
		out := string(renderComponent("does-not-exist"))
		if !strings.Contains(out, "unknown templ component") {
			t.Fatalf("expected fallback comment, got %q", out)
		}
	})
}
