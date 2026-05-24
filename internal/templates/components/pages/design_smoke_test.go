//go:build !headless && !lite

package pages

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"breadbox/internal/templates/components"
)

// TestDesignGalleryRenders is a smoke test: every DesignSections() entry
// must render without error, and the gallery itself must inline the
// section markup. Catches missing templ functions, prop mismatches, or
// templates that silently emit nothing — none of which the Go compiler
// catches but all of which would break the sandbox at runtime.
func TestDesignGalleryRenders(t *testing.T) {
	sections := DesignSections()
	if len(sections) == 0 {
		t.Fatal("DesignSections() returned empty slice")
	}

	// Build a set of known group slugs so we can assert every section
	// declares a real one (a typo would silently exile the section from
	// the sidebar).
	groupSlugs := map[string]bool{}
	for _, g := range DesignSectionGroups() {
		groupSlugs[g.Slug] = true
	}

	// Per-section render: each section's Render() must produce non-empty
	// output. Silent-empty would mean the templ function returned NopComponent.
	for _, sec := range sections {
		if sec.Slug == "" || sec.Title == "" {
			t.Errorf("section %+v has empty slug or title", sec)
		}
		if sec.Group == "" {
			t.Errorf("section %q has empty Group — every section must be assigned to a top-level group", sec.Slug)
		} else if !groupSlugs[sec.Group] {
			t.Errorf("section %q has unknown Group=%q (not in DesignSectionGroups)", sec.Slug, sec.Group)
		}
		if sec.Render == nil {
			t.Errorf("section %q has nil Render", sec.Slug)
			continue
		}
		var buf bytes.Buffer
		if err := sec.Render().Render(context.Background(), &buf); err != nil {
			t.Errorf("section %q: render error: %v", sec.Slug, err)
			continue
		}
		if buf.Len() < 50 {
			t.Errorf("section %q: render output too small (%d bytes) — likely NopComponent or stub", sec.Slug, buf.Len())
		}
	}

	// Gallery render: must include the anchor for every section's slug.
	var buf bytes.Buffer
	props := DesignGalleryProps{
		Sections:    sections,
		Breadcrumbs: []components.Breadcrumb{{Label: "Design system"}},
	}
	if err := DesignGallery(props).Render(context.Background(), &buf); err != nil {
		t.Fatalf("gallery render error: %v", err)
	}
	gallery := buf.String()
	for _, sec := range sections {
		if !strings.Contains(gallery, `id="`+sec.Slug+`"`) {
			t.Errorf("gallery missing anchor id=%q for section %q", sec.Slug, sec.Title)
		}
		if !strings.Contains(gallery, "/design/c/"+sec.Slug) {
			t.Errorf("gallery missing standalone link to /design/c/%s", sec.Slug)
		}
	}

	// FindDesignSection round-trips known slugs and rejects unknown.
	for _, sec := range sections {
		got, ok := FindDesignSection(sec.Slug)
		if !ok {
			t.Errorf("FindDesignSection(%q) returned !ok", sec.Slug)
		}
		if got.Title != sec.Title {
			t.Errorf("FindDesignSection(%q): title=%q want %q", sec.Slug, got.Title, sec.Title)
		}
	}
	if _, ok := FindDesignSection("not-a-real-slug"); ok {
		t.Error("FindDesignSection accepted unknown slug")
	}
}

// TestDesignComponentRenders confirms the standalone single-section page
// renders for every section without error.
func TestDesignComponentRenders(t *testing.T) {
	for _, sec := range DesignSections() {
		var buf bytes.Buffer
		props := DesignComponentProps{
			Section: sec,
			Breadcrumbs: []components.Breadcrumb{
				{Label: "Design system", Href: "/design"},
				{Label: sec.Title},
			},
		}
		if err := DesignComponent(props).Render(context.Background(), &buf); err != nil {
			t.Errorf("section %q standalone render error: %v", sec.Slug, err)
			continue
		}
		out := buf.String()
		if len(out) < 200 {
			t.Errorf("section %q standalone page too small (%d bytes)", sec.Slug, len(out))
		}
		// The breadcrumb back link should always be present.
		if !strings.Contains(out, `href="/design"`) {
			t.Errorf("section %q standalone page missing back link to /design", sec.Slug)
		}
	}
}
