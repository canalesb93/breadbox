package admin

import (
	"strings"
	"testing"
)

// legacyTagFields mirrors the shape of service.TagResponse /
// service.AdminTransactionTag — the two live callers that rendered the former
// tag_chip.html partial via {{template}}. The bridge must keep accepting both
// shapes via reflection so existing handler data maps don't need changes.
type legacyTagFields struct {
	Slug        string
	DisplayName string
	Color       *string
	Icon        *string
	// Unrelated fields that should be ignored.
	Lifecycle string
}

func TestTagChipFuncAcceptsStructByValueOrPointer(t *testing.T) {
	color := "#112233"
	icon := "tag"
	src := legacyTagFields{
		Slug:        "work",
		DisplayName: "Work",
		Color:       &color,
		Icon:        &icon,
	}

	for name, input := range map[string]any{
		"by value":   src,
		"by pointer": &src,
	} {
		t.Run(name, func(t *testing.T) {
			got := string(tagChipFunc(input))
			if !strings.Contains(got, `title="work"`) {
				t.Errorf("missing slug: %q", got)
			}
			if !strings.Contains(got, "--tag-color: #112233") {
				t.Errorf("missing color: %q", got)
			}
			if !strings.Contains(got, `data-lucide="tag"`) {
				t.Errorf("missing icon: %q", got)
			}
			if !strings.Contains(got, ">Work<") {
				t.Errorf("missing display name: %q", got)
			}
		})
	}
}

func TestTagChipSmFuncAppliesCompactClass(t *testing.T) {
	got := string(tagChipSmFunc(legacyTagFields{Slug: "x", DisplayName: "X"}))
	if !strings.Contains(got, "bb-tag bb-tag-sm") {
		t.Errorf("missing bb-tag-sm class: %q", got)
	}
}

func TestTagChipFuncToleratesNilAndUnknownShapes(t *testing.T) {
	if got := string(tagChipFunc(nil)); !strings.Contains(got, "bb-tag") {
		t.Errorf("nil input should still render an empty chip, got: %q", got)
	}
	// A type without the expected fields renders a chip with empty text.
	if got := string(tagChipFunc(42)); !strings.Contains(got, "bb-tag") {
		t.Errorf("non-struct input should render an empty chip, got: %q", got)
	}
}
