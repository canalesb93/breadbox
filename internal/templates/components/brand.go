//go:build !headless && !lite

package components

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed brand/*.svg
var brandFS embed.FS

var brandIcons = loadBrandIcons()

func loadBrandIcons() map[string]string {
	out := map[string]string{}
	entries, err := brandFS.ReadDir("brand")
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".svg") {
			continue
		}
		data, err := brandFS.ReadFile("brand/" + e.Name())
		if err != nil {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".svg")
		out[slug] = string(data)
	}
	return out
}

// renderBrandIcon mirrors renderLucideIcon: stamps the resolved class
// list into the embedded <svg> at render time. The `brand` class is
// load-bearing — input.css scopes `svg.brand` for the same
// pointer-events neutralisation as `svg.lucide`. Multi-color brand
// marks (Teller) ignore currentColor by design; monochrome marks
// (Plaid, GitHub, Anthropic, Claude mono, MCP) inherit it.
func renderBrandIcon(slug, class string) string {
	raw, ok := brandIcons[slug]
	if !ok {
		return ""
	}
	merged := "brand brand-" + slug
	if class != "" {
		merged += " " + class
	}
	return strings.Replace(raw, "<svg ", fmt.Sprintf(`<svg class=%q `, merged), 1)
}
