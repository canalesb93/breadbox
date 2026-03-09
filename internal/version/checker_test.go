package version

import (
	"testing"
)

func TestIsNewer(t *testing.T) {
	tests := []struct {
		name    string
		latest  string
		current string
		want    bool
	}{
		{"patch bump", "1.0.1", "1.0.0", true},
		{"minor bump", "1.1.0", "1.0.0", true},
		{"major bump", "2.0.0", "1.0.0", true},
		{"equal versions", "1.0.0", "1.0.0", false},
		{"current is newer patch", "1.0.0", "1.0.1", false},
		{"current is newer minor", "1.0.0", "1.1.0", false},
		{"current is newer major", "1.0.0", "2.0.0", false},
		{"multi-digit", "1.10.0", "1.9.0", true},
		{"complex version", "2.3.14", "2.3.13", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNewer(tt.latest, tt.current)
			if got != tt.want {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

func TestParseSemver(t *testing.T) {
	tests := []struct {
		name string
		v    string
		want []int
	}{
		{"simple", "1.2.3", []int{1, 2, 3}},
		{"with v prefix", "v1.2.3", []int{1, 2, 3}},
		{"prerelease stripped", "1.2.3-beta1", []int{1, 2, 3}},
		{"build metadata stripped", "1.2.3+build456", []int{1, 2, 3}},
		{"zero version", "0.0.0", []int{0, 0, 0}},
		{"large numbers", "100.200.300", []int{100, 200, 300}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSemver(tt.v)
			if got == nil {
				t.Fatal("parseSemver returned nil")
			}
			for i := 0; i < 3; i++ {
				if got[i] != tt.want[i] {
					t.Errorf("parseSemver(%q)[%d] = %d, want %d", tt.v, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseSemverInvalid(t *testing.T) {
	tests := []struct {
		name string
		v    string
	}{
		{"empty", ""},
		{"single number", "1"},
		{"two parts", "1.2"},
		{"non-numeric", "a.b.c"},
		{"partial non-numeric", "1.2.abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSemver(tt.v)
			if got != nil {
				t.Errorf("parseSemver(%q) = %v, want nil", tt.v, got)
			}
		})
	}
}

func TestIsNewerInvalidVersions(t *testing.T) {
	// Invalid versions should return false (safe default).
	if isNewer("not-semver", "1.0.0") {
		t.Error("invalid latest should return false")
	}
	if isNewer("1.0.0", "not-semver") {
		t.Error("invalid current should return false")
	}
	if isNewer("bad", "also-bad") {
		t.Error("both invalid should return false")
	}
}
