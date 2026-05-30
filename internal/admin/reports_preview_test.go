//go:build !headless && !lite

package admin

import (
	"strings"
	"testing"
)

// TestReportPreview covers the markdown→plain-text stripping used for the
// inbox-row preview. The headline cases are the ones the blanket [*_~]
// strip used to corrupt: snake_case identifiers and bare arithmetic must
// survive, while real emphasis runs are unwrapped to their inner text.
func TestReportPreview(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "Nothing to flag this week.", "Nothing to flag this week."},
		{"snake_case survives", "The file_name field maps to cost_basis.", "The file_name field maps to cost_basis."},
		{"bare arithmetic survives", "Roughly 5*4 = 20 charges.", "Roughly 5*4 = 20 charges."},
		{"bold unwrapped", "This is **important** today.", "This is important today."},
		{"italic unwrapped", "This is *subtle* emphasis.", "This is subtle emphasis."},
		{"underscore italic unwrapped", "A _quiet_ note.", "A quiet note."},
		{"strikethrough unwrapped", "Was ~~wrong~~ now right.", "Was wrong now right."},
		{"inline code kept", "Run `make css` first.", "Run make css first."},
		{"heading marker stripped", "## Summary\n\nAll good.", "Summary All good."},
		{"list markers stripped", "- one\n- two\n- three", "one two three"},
		{"link text kept, url dropped", "See [the docs](https://example.com) now.", "See the docs now."},
		{"blockquote marker stripped", "> Net impact is high.", "Net impact is high."},
		{"whitespace collapsed", "a\n\n\tb   c", "a b c"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reportPreview(c.in); got != c.want {
				t.Errorf("reportPreview(%q) = %q; want %q", c.in, got, c.want)
			}
		})
	}
}

// TestReportPreviewTruncates verifies the snippet is clipped on a word
// boundary with an ellipsis and never exceeds the budget mid-word.
func TestReportPreviewTruncates(t *testing.T) {
	long := strings.Repeat("alpha bravo charlie delta echo ", 20) // ~600 chars
	got := reportPreview(long)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected an ellipsis suffix, got %q", got)
	}
	// Budget is ~140 runes plus the ellipsis; allow a little slack but
	// reject anything near the full input length.
	if n := len([]rune(got)); n > 150 {
		t.Fatalf("preview too long: %d runes", n)
	}
	if strings.Contains(strings.TrimSuffix(got, "…"), "  ") {
		t.Fatalf("preview should be whitespace-collapsed: %q", got)
	}
}
