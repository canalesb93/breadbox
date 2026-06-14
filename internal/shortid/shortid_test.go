package shortid

import (
	"testing"
)

func TestGenerate(t *testing.T) {
	id, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if len(id) != Length {
		t.Fatalf("expected length %d, got %d: %q", Length, len(id), id)
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
			t.Fatalf("invalid character %q in %q", c, id)
		}
	}
}

func TestGenerateUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 10000; i++ {
		id, err := Generate()
		if err != nil {
			t.Fatalf("Generate() error: %v", err)
		}
		if seen[id] {
			t.Fatalf("duplicate ID generated: %q after %d iterations", id, i)
		}
		seen[id] = true
	}
}

// TestGenerateLeadingCharDistribution guards against modulo bias. 6 random
// bytes span [0, 2^48), which exceeds the 62^8 code space, so a naive reduction
// would over-represent IDs whose leading character maps to a low index (the
// first ~18 of 62 symbols would be ~2x as likely). Generate rejection-samples
// to keep the distribution uniform; here we assert the leading-character low
// bucket stays close to the uniform expectation. With 60k samples the unbiased
// fraction is ~0.290 with tiny noise, while the buggy reduction skews it to
// ~0.452 — far outside the tolerance below.
func TestGenerateLeadingCharDistribution(t *testing.T) {
	const samples = 60000
	const lowCut = 18 // alphabet indices [0,18) are the over-represented region under bias

	indexOf := func(c byte) int {
		for i := 0; i < len(alphabet); i++ {
			if alphabet[i] == c {
				return i
			}
		}
		return -1
	}

	lowCount := 0
	for i := 0; i < samples; i++ {
		id, err := Generate()
		if err != nil {
			t.Fatalf("Generate() error: %v", err)
		}
		idx := indexOf(id[0])
		if idx < 0 {
			t.Fatalf("leading char %q not in alphabet (%q)", id[0], id)
		}
		if idx < lowCut {
			lowCount++
		}
	}

	got := float64(lowCount) / float64(samples)
	want := float64(lowCut) / 62 // ~0.290 when uniform
	if got < want-0.03 || got > want+0.03 {
		t.Errorf("leading-char low-bucket fraction = %.4f, want ~%.4f (modulo bias?)", got, want)
	}
}

func TestIsShortID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"k7Xm9pQ2", true},
		{"ABCD1234", true},
		{"abcdefgh", true},
		{"12345678", true},
		// Too short/long
		{"abc", false},
		{"abcdefghi", false},
		{"", false},
		// Invalid characters
		{"abc-efgh", false},
		{"abc_efgh", false},
		{"abc efgh", false},
		// UUID
		{"9466ab98-0de2-41a0-847b-6740bb519cdc", false},
	}
	for _, tt := range tests {
		if got := IsShortID(tt.input); got != tt.want {
			t.Errorf("IsShortID(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
