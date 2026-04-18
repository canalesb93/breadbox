package avatar

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
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

func TestProcessUpload_Success(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		encode      func(*bytes.Buffer, image.Image) error
		width       int
		height      int
	}{
		{
			name:        "square png",
			contentType: "image/png",
			encode:      func(b *bytes.Buffer, img image.Image) error { return png.Encode(b, img) },
			width:       300,
			height:      300,
		},
		{
			name:        "landscape jpeg",
			contentType: "image/jpeg",
			encode:      func(b *bytes.Buffer, img image.Image) error { return jpeg.Encode(b, img, nil) },
			width:       400,
			height:      200,
		},
		{
			name:        "portrait gif",
			contentType: "image/gif",
			encode:      func(b *bytes.Buffer, img image.Image) error { return gif.Encode(b, img, nil) },
			width:       200,
			height:      400,
		},
		{
			name:        "small upscale png",
			contentType: "image/png",
			encode:      func(b *bytes.Buffer, img image.Image) error { return png.Encode(b, img) },
			width:       64,
			height:      64,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := makeSolidImage(tt.width, tt.height, color.RGBA{R: 200, G: 100, B: 50, A: 255})
			var buf bytes.Buffer
			if err := tt.encode(&buf, src); err != nil {
				t.Fatalf("encode source: %v", err)
			}

			out, outType, err := ProcessUpload(buf.Bytes(), tt.contentType)
			if err != nil {
				t.Fatalf("ProcessUpload: %v", err)
			}
			if outType != "image/png" {
				t.Errorf("output content-type = %q, want image/png", outType)
			}
			decoded, err := png.Decode(bytes.NewReader(out))
			if err != nil {
				t.Fatalf("decode output: %v", err)
			}
			b := decoded.Bounds()
			if b.Dx() != targetSize || b.Dy() != targetSize {
				t.Errorf("output size = %dx%d, want %dx%d", b.Dx(), b.Dy(), targetSize, targetSize)
			}
		})
	}
}

func TestCenterCrop(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		wantSize      int
	}{
		{"already square", 100, 100, 100},
		{"landscape crops to height", 300, 150, 150},
		{"portrait crops to width", 150, 300, 150},
		{"tall strip", 50, 500, 50},
		{"wide strip", 500, 50, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := makeSolidImage(tt.width, tt.height, color.RGBA{A: 255})
			got := centerCrop(src)
			b := got.Bounds()
			if b.Dx() != tt.wantSize || b.Dy() != tt.wantSize {
				t.Errorf("cropped size = %dx%d, want %dx%d", b.Dx(), b.Dy(), tt.wantSize, tt.wantSize)
			}
		})
	}
}

func TestCenterCrop_FallbackNonSubImager(t *testing.T) {
	// nonSubImager wraps an image.Image but does not implement SubImage,
	// forcing centerCrop's manual-copy fallback path.
	src := nonSubImager{makeSolidImage(200, 100, color.RGBA{R: 255, A: 255})}
	got := centerCrop(src)
	b := got.Bounds()
	if b.Dx() != 100 || b.Dy() != 100 {
		t.Errorf("fallback cropped size = %dx%d, want 100x100", b.Dx(), b.Dy())
	}
}

// makeSolidImage returns an opaque image of the given dimensions filled with c.
func makeSolidImage(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

type nonSubImager struct {
	image.Image
}
