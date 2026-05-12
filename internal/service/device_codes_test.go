package service

import (
	"strings"
	"testing"
)

// Unit-level coverage for the user-code generator and normalization
// helper. These have no DB dependency, so they live in the regular
// (unit) test suite and run on `go test ./...`.

func TestGenerateUserCode_Format(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		code, err := generateUserCode()
		if err != nil {
			t.Fatalf("generateUserCode: %v", err)
		}
		if len(code) != 8 {
			t.Fatalf("len(%q) = %d, want 8", code, len(code))
		}
		for _, r := range code {
			if !strings.ContainsRune(userCodeAlphabet, r) {
				t.Fatalf("code %q contains glyph %q outside alphabet %q", code, r, userCodeAlphabet)
			}
		}
		// 26^8 ≈ 208 billion — 200 iterations should never collide.
		if seen[code] {
			t.Fatalf("collision after %d iterations: %q", i, code)
		}
		seen[code] = true
	}
}

func TestUserCodeAlphabet_NoAmbiguousGlyphs(t *testing.T) {
	bad := []rune{'0', 'O', '1', 'I', 'L', 'U', 'V', 'S', '5'}
	for _, b := range bad {
		if strings.ContainsRune(userCodeAlphabet, b) {
			t.Errorf("ambiguous glyph %q must not be in userCodeAlphabet", b)
		}
	}
}

func TestFormatUserCode(t *testing.T) {
	cases := map[string]string{
		"ABCDEFGH": "ABCD-EFGH",
		"":         "",
		"ABC":      "ABC",
		"XXXXXXXX": "XXXX-XXXX",
	}
	for in, want := range cases {
		if got := FormatUserCode(in); got != want {
			t.Errorf("FormatUserCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeUserCode(t *testing.T) {
	cases := map[string]string{
		"ABCD-EFGH":  "ABCDEFGH",
		"abcd-efgh":  "ABCDEFGH",
		" ABCDEFGH ": "ABCDEFGH",
		"ABCDEFGH":   "ABCDEFGH",
		"abc-defgh":  "ABCDEFGH",
		"ABCD EFGH":  "ABCDEFGH",
		"":           "",
		"ABC":        "",
		"ABCDEFG":    "",
		"ABCDEFGHI":  "",
		// 0 / 1 / O / I / L not in alphabet — reject.
		"0BCDEFGH": "",
		"1BCDEFGH": "",
		"OBCDEFGH": "",
		"IBCDEFGH": "",
	}
	for in, want := range cases {
		if got := normalizeUserCode(in); got != want {
			t.Errorf("normalizeUserCode(%q) = %q, want %q", in, got, want)
		}
	}
}
