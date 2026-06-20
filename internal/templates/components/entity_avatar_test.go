package components

import (
	"strings"
	"testing"
)

func TestEntityAvatarLetters(t *testing.T) {
	cases := map[string]string{
		"Amazon":            "AM",
		"Netflix":           "NE",
		"John Smith":        "JS",
		"acme corp":         "AC",
		"  spaced  out  ":   "SO",
		"x":                 "X",
		"4Front":            "4F",
		"":                  "?",
		"   ":               "?",
		"!!!":               "?",
		"Spotify Premium":   "SP",
		"a b c":             "AB", // first two word-initials only
		"E*Trade":           "ET", // symbol skipped inside the word
	}
	for in, want := range cases {
		if got := entityAvatarLetters(in); got != want {
			t.Errorf("entityAvatarLetters(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEntityAvatarHueDeterministic(t *testing.T) {
	// Same seed → same hue, every time.
	if a, b := entityAvatarHue("Amazon"), entityAvatarHue("Amazon"); a != b {
		t.Fatalf("hue not stable: %d != %d", a, b)
	}
	// Case/whitespace-insensitive (the seed is normalized).
	if a, b := entityAvatarHue("Amazon"), entityAvatarHue("  amazon "); a != b {
		t.Errorf("hue not normalized: %d != %d", a, b)
	}
	// Always in range.
	for _, s := range []string{"", "Netflix", "ACME Corp", "🍞", "long name with words"} {
		if h := entityAvatarHue(s); h < 0 || h >= 360 {
			t.Errorf("entityAvatarHue(%q) = %d, out of [0,360)", s, h)
		}
	}
	// Distinct names generally get distinct hues (sanity, not a guarantee).
	if entityAvatarHue("Amazon") == entityAvatarHue("Netflix") {
		t.Errorf("expected Amazon and Netflix to differ in hue")
	}
}

func TestEntityAvatarSeedFallsBackToName(t *testing.T) {
	if got := entityAvatarSeed(EntityAvatarProps{Name: "Amazon"}); got != "Amazon" {
		t.Errorf("seed should fall back to Name, got %q", got)
	}
	if got := entityAvatarSeed(EntityAvatarProps{Name: "Amazon", Seed: "cp_123"}); got != "cp_123" {
		t.Errorf("seed should prefer Seed, got %q", got)
	}
}

func TestEntityAvatarTileClass(t *testing.T) {
	// sm tile is rounded-lg + 36px; md is rounded-xl + 40px; empty size lets the
	// caller's extra classes drive sizing (header fill).
	if got := entityAvatarTileClass(EntityAvatarSizeSM, ""); !strings.Contains(got, "w-9 h-9") || !strings.Contains(got, "rounded-lg") {
		t.Errorf("sm tile class = %q", got)
	}
	if got := entityAvatarTileClass(EntityAvatarSizeMD, ""); !strings.Contains(got, "w-10 h-10") || !strings.Contains(got, "rounded-xl") {
		t.Errorf("md tile class = %q", got)
	}
	if got := entityAvatarTileClass("", "w-full h-full"); !strings.Contains(got, "w-full h-full") {
		t.Errorf("header tile class should carry extra, = %q", got)
	}
}
