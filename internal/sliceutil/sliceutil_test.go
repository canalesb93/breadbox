package sliceutil

import (
	"reflect"
	"testing"
)

func TestContains(t *testing.T) {
	cases := []struct {
		name   string
		slice  []string
		target string
		want   bool
	}{
		{"present exact", []string{"a", "b", "c"}, "b", true},
		{"absent", []string{"a", "b"}, "z", false},
		{"case mismatch", []string{"Alpha"}, "alpha", false},
		{"empty slice", nil, "x", false},
		{"empty target present", []string{"", "a"}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Contains(tc.slice, tc.target); got != tc.want {
				t.Fatalf("Contains(%v, %q) = %v, want %v", tc.slice, tc.target, got, tc.want)
			}
		})
	}
}

func TestContainsFold(t *testing.T) {
	cases := []struct {
		name   string
		slice  []string
		target string
		want   bool
	}{
		{"case-insensitive match", []string{"Alpha", "Beta"}, "alpha", true},
		{"absent", []string{"Alpha"}, "gamma", false},
		{"empty target never matches", []string{"", "a"}, "", false},
		{"empty slice", nil, "x", false},
		{"unicode fold", []string{"Éclair"}, "éclair", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ContainsFold(tc.slice, tc.target); got != tc.want {
				t.Fatalf("ContainsFold(%v, %q) = %v, want %v", tc.slice, tc.target, got, tc.want)
			}
		})
	}
}

func TestIndexFold(t *testing.T) {
	cases := []struct {
		name   string
		slice  []string
		target string
		want   int
	}{
		{"first match", []string{"Alpha", "Beta"}, "alpha", 0},
		{"later match", []string{"Alpha", "Beta"}, "BETA", 1},
		{"absent", []string{"Alpha"}, "gamma", -1},
		{"empty slice", nil, "x", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IndexFold(tc.slice, tc.target); got != tc.want {
				t.Fatalf("IndexFold(%v, %q) = %d, want %d", tc.slice, tc.target, got, tc.want)
			}
		})
	}
}

func TestDropFold(t *testing.T) {
	cases := []struct {
		name   string
		slice  []string
		target string
		want   []string
	}{
		{"drops single match", []string{"a", "B", "c"}, "b", []string{"a", "c"}},
		{"drops all matches", []string{"x", "X", "y", "x"}, "X", []string{"y"}},
		{"no match returns equivalent", []string{"a", "b"}, "z", []string{"a", "b"}},
		{"empty slice", nil, "x", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Clone so we can assert behavior without cross-case aliasing via shared backing storage.
			input := append([]string(nil), tc.slice...)
			got := DropFold(input, tc.target)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("DropFold(%v, %q) = %v, want %v", tc.slice, tc.target, got, tc.want)
			}
		})
	}
}
