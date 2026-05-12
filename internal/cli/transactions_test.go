package cli

import (
	"reflect"
	"testing"
)

// TestExpandCSV asserts repeated --tag flags and comma-joined values
// both produce the same canonical slice. The transactions list/count/
// summary handlers rely on this for their filter parsing.
func TestExpandCSV(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty input", nil, []string{}},
		{"single value", []string{"foo"}, []string{"foo"}},
		{"repeated flag form", []string{"a", "b"}, []string{"a", "b"}},
		{"comma joined", []string{"a,b,c"}, []string{"a", "b", "c"}},
		{"mixed forms with whitespace", []string{"a, b", "c"}, []string{"a", "b", "c"}},
		{"empty fragments dropped", []string{"a,,b"}, []string{"a", "b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := expandCSV(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestTruncate covers the comment-content truncation helper. Pure plumbing
// but easy to regress when someone tweaks the ellipsis length.
func TestTruncate(t *testing.T) {
	if got := truncate("hello", 80); got != "hello" {
		t.Errorf("short string changed: got %q", got)
	}
	if got := truncate("abcdefghij", 6); got != "abc..." {
		t.Errorf("long string ellipsis wrong: got %q want %q", got, "abc...")
	}
	if got := truncate("abc", 2); got != "ab" {
		t.Errorf("tiny limit should hard cut: got %q want %q", got, "ab")
	}
}
