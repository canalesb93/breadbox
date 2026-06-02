//go:build !headless && !lite

package admin

import (
	"os"
	"regexp"
	"testing"

	"breadbox/internal/templates/components"
)

// TestPageIDFromHref locks the href → nav-page-id extraction the topbar
// uses to resolve a trail's section group.
func TestPageIDFromHref(t *testing.T) {
	cases := map[string]string{
		"/connections?tab=links": "connections",
		"/workflows/runs":        "workflows",
		"/transactions":          "transactions",
		"/agents/definitions":    "agents",
		"/household/123/edit":    "household",
		"":                       "",
	}
	for href, want := range cases {
		if got := pageIDFromHref(href); got != want {
			t.Errorf("pageIDFromHref(%q) = %q, want %q", href, got, want)
		}
	}
}

// TestTopbarSectionLabel verifies the topbar section is derived from the
// trail's root crumb (so it never contradicts the path), and falls back to
// the current page's section on trail-less list pages.
func TestTopbarSectionLabel(t *testing.T) {
	crumb := func(label, href string) components.Breadcrumb {
		return components.Breadcrumb{Label: label, Href: href}
	}
	cases := []struct {
		name        string
		currentPage string
		items       []components.Breadcrumb
		want        string
	}{
		{"list page falls back to current section", "transactions", nil, "Overview"},
		{"detail derives section from trail root", "transactions",
			[]components.Breadcrumb{crumb("Connections", "/connections"), crumb("Chase", "/connections/x"), crumb("Account", "")},
			"System"},
		{"agent run trail roots at workflows", "workflows",
			[]components.Breadcrumb{crumb("Runs", "/workflows/runs"), crumb("Agent", ""), crumb("ab12", "")},
			"Manage"},
		{"sectionless current crumb falls back to current page", "design",
			[]components.Breadcrumb{crumb("Design system", "")}, ""},
	}
	for _, c := range cases {
		if got := topbarSectionLabel(c.currentPage, c.items); got != c.want {
			t.Errorf("%s: topbarSectionLabel(%q, …) = %q, want %q", c.name, c.currentPage, got, c.want)
		}
	}
}

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
