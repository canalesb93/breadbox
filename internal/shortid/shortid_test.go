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
