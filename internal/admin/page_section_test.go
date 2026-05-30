//go:build !headless && !lite

package admin

import (
	"os"
	"regexp"
	"testing"
)

// TestPageSectionCoversNav guards the one coupling pageSectionLabel can't
// express in code: the topbar breadcrumb's section is derived here, but the
// section groupings themselves live as markup in components.Nav (nav.templ).
// If a nav destination is added (or moved) without a matching
// pageSectionLabel entry, the breadcrumb silently renders with no section
// prefix. This reads the nav source directly and fails loudly instead.
func TestPageSectionCoversNav(t *testing.T) {
	src, err := os.ReadFile("../templates/components/nav.templ")
	if err != nil {
		t.Fatalf("read nav.templ: %v", err)
	}
	// Every sidebar link sets its active state via navActive(p.CurrentPage, "<id>").
	re := regexp.MustCompile(`navActive\(p\.CurrentPage, "([^"]+)"\)`)
	matches := re.FindAllStringSubmatch(string(src), -1)
	if len(matches) == 0 {
		t.Fatal("no navActive(p.CurrentPage, ...) page ids found in nav.templ — the regex or nav structure changed; update this test")
	}
	seen := map[string]bool{}
	for _, m := range matches {
		id := m[1]
		if seen[id] {
			continue
		}
		seen[id] = true
		if got := pageSectionLabel(id); got == "" {
			t.Errorf("nav page %q has no section in pageSectionLabel — the topbar breadcrumb will render without a section prefix; add %q to pageSectionLabel", id, id)
		}
	}
}
