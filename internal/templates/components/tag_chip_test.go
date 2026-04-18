package components

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func renderToString(t *testing.T, data TagChipData, small bool) string {
	t.Helper()
	var buf bytes.Buffer
	var err error
	if small {
		err = TagChipSm(data).Render(context.Background(), &buf)
	} else {
		err = TagChip(data).Render(context.Background(), &buf)
	}
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	return buf.String()
}

func ptr(s string) *string { return &s }

func TestTagChipRendersCanonicalMarkup(t *testing.T) {
	got := renderToString(t, TagChipData{
		Slug:        "needs-review",
		DisplayName: "Needs Review",
		Color:       ptr("#ff0066"),
		Icon:        ptr("eye"),
	}, false)

	// Core class + pill shape.
	if !strings.Contains(got, `class="bb-tag"`) {
		t.Errorf("missing bb-tag class: %q", got)
	}
	// Stable slug title for JS hooks in transactions.html.
	if !strings.Contains(got, `title="needs-review"`) {
		t.Errorf("missing slug title: %q", got)
	}
	// Color inline style.
	if !strings.Contains(got, `--tag-color: #ff0066`) {
		t.Errorf("missing color style: %q", got)
	}
	// Icon.
	if !strings.Contains(got, `data-lucide="eye"`) {
		t.Errorf("missing icon: %q", got)
	}
	// Display name.
	if !strings.Contains(got, `>Needs Review<`) {
		t.Errorf("missing display name: %q", got)
	}
}

func TestTagChipSmAddsCompactClass(t *testing.T) {
	got := renderToString(t, TagChipData{Slug: "x", DisplayName: "X"}, true)
	if !strings.Contains(got, "bb-tag bb-tag-sm") {
		t.Errorf("missing bb-tag-sm class: %q", got)
	}
}

func TestTagChipSkipsEmptyColorAndIcon(t *testing.T) {
	// Nil pointers — most-common case for tags without theme customization.
	got := renderToString(t, TagChipData{Slug: "plain", DisplayName: "Plain"}, false)
	if strings.Contains(got, "--tag-color") {
		t.Errorf("unexpected color style when nil: %q", got)
	}
	if strings.Contains(got, "data-lucide") {
		t.Errorf("unexpected icon when nil: %q", got)
	}

	// Empty-string pointers should also be skipped — matches legacy partial.
	empty := ""
	got = renderToString(t, TagChipData{
		Slug:        "plain",
		DisplayName: "Plain",
		Color:       &empty,
		Icon:        &empty,
	}, false)
	if strings.Contains(got, "--tag-color") {
		t.Errorf("unexpected color style when empty: %q", got)
	}
	if strings.Contains(got, "data-lucide") {
		t.Errorf("unexpected icon when empty: %q", got)
	}
}
