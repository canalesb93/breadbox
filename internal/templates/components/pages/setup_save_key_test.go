//go:build !headless && !lite

package pages

import (
	"context"
	"strings"
	"testing"
)

// TestMaskedDots verifies the SSR masking helper produces one bullet per
// input rune. The template renders this initial state so a slow JS load
// doesn't briefly flash the real key before swapping in the masked form.
func TestMaskedDots(t *testing.T) {
	cases := []struct{ in string }{
		{""},
		{"abcd"},
		{"09baf2e29942748f42a9a3d5d3a0afb897eabce85735caea8895bc7898d793e7"}, // 64 hex
	}
	for _, tc := range cases {
		got := maskedDots(tc.in)
		if got == tc.in && tc.in != "" {
			t.Errorf("maskedDots(%q) returned input unchanged", tc.in)
		}
		// One bullet per rune (●), no leakage of real characters.
		wantRunes := len([]rune(tc.in))
		gotRunes := len([]rune(got))
		if gotRunes != wantRunes {
			t.Errorf("maskedDots(%q): got %d runes, want %d", tc.in, gotRunes, wantRunes)
		}
		for _, r := range got {
			if r != '●' {
				t.Errorf("maskedDots(%q) leaked non-bullet rune %q", tc.in, r)
				break
			}
		}
	}
}

// TestSaveKeyRendersMaskedNotPlain ensures the rendered template never
// emits the plaintext key inside the visible <span data-key-text>. The
// reveal toggle is a client-side swap; the SSR payload must start masked.
func TestSaveKeyRendersMaskedNotPlain(t *testing.T) {
	const key = "deadbeefcafebabe1234567890abcdef" + "deadbeefcafebabe1234567890abcdef"
	props := SaveKeyProps{
		PageTitle:     "Save encryption key",
		EncryptionKey: key,
	}
	var buf strings.Builder
	if err := SaveKey(props).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	// The key must NOT appear inside the visible data-key-text span.
	visibleStart := strings.Index(out, `<span data-key-text>`)
	if visibleStart < 0 {
		t.Fatal("rendered output missing data-key-text span")
	}
	visibleEnd := strings.Index(out[visibleStart:], `</span>`)
	if visibleEnd < 0 {
		t.Fatal("rendered output missing closing </span> for data-key-text")
	}
	visible := out[visibleStart : visibleStart+visibleEnd]
	if strings.Contains(visible, key) {
		t.Errorf("plaintext key leaked into visible span: %q", visible)
	}
	// The key SHOULD appear in the data-key-value attribute so JS can swap it in on reveal.
	if !strings.Contains(out, `data-key-value="`+key+`"`) {
		t.Error("rendered output missing data-key-value attribute carrying the key")
	}
}
