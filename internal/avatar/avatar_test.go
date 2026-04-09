package avatar

import (
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
