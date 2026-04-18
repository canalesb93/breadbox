package avatar

import (
	"math"
	"strings"
	"testing"
)

func TestGenerateSVG_Deterministic(t *testing.T) {
	svg1 := GenerateSVG("test-seed", 256)
	svg2 := GenerateSVG("test-seed", 256)

	if string(svg1) != string(svg2) {
		t.Error("expected identical SVGs for same seed")
	}
}

func TestGenerateSVG_DifferentSeeds(t *testing.T) {
	svg1 := GenerateSVG("user-1", 256)
	svg2 := GenerateSVG("user-2", 256)

	if string(svg1) == string(svg2) {
		t.Error("expected different SVGs for different seeds")
	}
}

func TestGenerateSVG_ValidSVG(t *testing.T) {
	svg := string(GenerateSVG("test", 256))

	if !strings.HasPrefix(svg, "<svg") {
		t.Error("expected SVG to start with <svg")
	}
	if !strings.HasSuffix(svg, "</svg>") {
		t.Error("expected SVG to end with </svg>")
	}
	if !strings.Contains(svg, `viewBox="0 0 256 256"`) {
		t.Error("expected viewBox attribute")
	}
}

func TestHslToHex(t *testing.T) {
	tests := []struct {
		name    string
		h, s, l float64
		want    string
	}{
		{"black", 0, 0, 0, "#000000"},
		{"white", 0, 0, 100, "#ffffff"},
		{"mid gray", 0, 0, 50, "#808080"},
		{"pure red", 0, 100, 50, "#ff0000"},
		{"pure yellow", 60, 100, 50, "#ffff00"},
		{"pure green", 120, 100, 50, "#00ff00"},
		{"pure cyan", 180, 100, 50, "#00ffff"},
		{"pure blue", 240, 100, 50, "#0000ff"},
		{"pure magenta", 300, 100, 50, "#ff00ff"},
		{"hue 359 is near red", 359, 100, 50, "#ff0004"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hslToHex(tt.h, tt.s, tt.l)
			if got != tt.want {
				t.Errorf("hslToHex(%v, %v, %v) = %q, want %q", tt.h, tt.s, tt.l, got, tt.want)
			}
		})
	}
}

func TestHslToHex_AlwaysSevenChars(t *testing.T) {
	// Sweep the hue/sat/lit space and ensure output is always a 7-char #rrggbb string.
	for h := 0.0; h < 360; h += 37 {
		for s := 0.0; s <= 100; s += 25 {
			for l := 0.0; l <= 100; l += 25 {
				got := hslToHex(h, s, l)
				if len(got) != 7 || got[0] != '#' {
					t.Fatalf("hslToHex(%v,%v,%v) = %q, want 7-char hex", h, s, l, got)
				}
			}
		}
	}
}

func TestClampOffset(t *testing.T) {
	const s = 100.0

	tests := []struct {
		name      string
		hashByte  byte
		r, maxR   float64
		want      float64
		tolerance float64
	}{
		{
			name:     "radius exceeds bounds returns zero",
			hashByte: 200, r: 60, maxR: 50,
			want: 0,
		},
		{
			name:     "raw within limit passes through",
			hashByte: 0, r: 5, maxR: 50, // raw = -20, limit = 45 → -20
			want: -20, tolerance: 0.01,
		},
		{
			name:     "raw above positive limit is clamped",
			hashByte: 255, r: 45, maxR: 50, // raw ≈ +20, limit = 5
			want: 5, tolerance: 0.01,
		},
		{
			name:     "raw below negative limit is clamped",
			hashByte: 0, r: 45, maxR: 50, // raw = -20, limit = 5
			want: -5, tolerance: 0.01,
		},
		{
			name:     "midpoint byte yields near-zero offset",
			hashByte: 128, r: 20, maxR: 50,
			want: 0, tolerance: 0.2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampOffset(tt.hashByte, s, tt.r, tt.maxR)
			if math.Abs(got-tt.want) > tt.tolerance {
				t.Errorf("clampOffset(%v, %v, %v, %v) = %v, want %v (±%v)",
					tt.hashByte, s, tt.r, tt.maxR, got, tt.want, tt.tolerance)
			}
		})
	}
}

func TestClampOffset_StaysWithinMaxR(t *testing.T) {
	// For any byte value and valid geometry, the resulting circle must fit within maxR.
	const s = 256.0
	const maxR = 120.0
	for b := 0; b < 256; b++ {
		for _, r := range []float64{10, 40, 80, 119} {
			off := clampOffset(byte(b), s, r, maxR)
			if math.Abs(off)+r > maxR+1e-9 {
				t.Fatalf("clampOffset(%d,%v,%v,%v)=%v: |off|+r=%v exceeds maxR=%v",
					b, s, r, maxR, off, math.Abs(off)+r, maxR)
			}
		}
	}
}

func TestProcessUpload_UnsupportedType(t *testing.T) {
	_, _, err := ProcessUpload([]byte("not an image"), "image/webp")
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}

func TestProcessUpload_CorruptImage(t *testing.T) {
	_, _, err := ProcessUpload([]byte("corrupt data"), "image/png")
	if err == nil {
		t.Error("expected error for corrupt image data")
	}
}
