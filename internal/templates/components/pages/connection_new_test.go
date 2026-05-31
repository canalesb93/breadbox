//go:build !headless && !lite

package pages

import (
	"context"
	"strings"
	"testing"
)

// TestConnectionNewContinueIconRenders guards the Continue button on
// /connections/new against the templ footgun where an "@components.LucideIcon"
// call placed after text on the same line is parsed as a literal text node
// instead of a component invocation — which leaked the raw templ source into
// the rendered button. The icon must render as a real arrow-right SVG.
func TestConnectionNewContinueIconRenders(t *testing.T) {
	var sb strings.Builder
	p := ConnectionNewProps{
		HasPlaid: true,
		Users:    []ConnectionNewUser{{ID: "u1", Name: "Alice"}},
	}
	if err := ConnectionNew(p).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := sb.String()
	if strings.Contains(out, "@components.LucideIcon") {
		t.Fatalf("rendered output still leaks literal templ source for the icon")
	}
	if !strings.Contains(out, "lucide-arrow-right") {
		t.Fatalf("rendered output missing the arrow-right svg icon")
	}
}
