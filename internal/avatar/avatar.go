// Package avatar generates deterministic SVG identicons and processes uploaded images.
package avatar

import (
	"crypto/sha256"
	"fmt"
	"math"
	"strings"
)

// GenerateSVG produces a deterministic SVG identicon from the given seed string.
// The seed is typically a user UUID. Size controls the SVG viewBox dimensions.
// The pattern is designed to look good inside circular containers (border-radius: 50%).
func GenerateSVG(seed string, size int) []byte {
	hash := sha256.Sum256([]byte(seed))

	// Pick two complementary colors from hash for a richer palette.
	hue1 := float64(uint16(hash[0])<<8|uint16(hash[1])) / 65535.0 * 360.0
	sat1 := 50.0 + float64(hash[2])/255.0*15.0 // 50-65%
	lit1 := 50.0 + float64(hash[3])/255.0*12.0 // 50-62%

	// Second color: offset hue by 30-90 degrees for harmony.
	hueOffset := 30.0 + float64(hash[4])/255.0*60.0
	hue2 := math.Mod(hue1+hueOffset, 360)
	sat2 := 40.0 + float64(hash[5])/255.0*20.0
	lit2 := 55.0 + float64(hash[6])/255.0*15.0

	fg1 := hslToHex(hue1, sat1, lit1)
	fg2 := hslToHex(hue2, sat2, lit2)
	bg := hslToHex(hue1, sat1*0.25, 94.0)

	s := float64(size)
	center := s / 2
	radius := s / 2

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`, size, size, size, size)

	// Circular clip path — ensures the pattern fills the circle cleanly.
	fmt.Fprintf(&b, `<defs><clipPath id="c"><circle cx="%.0f" cy="%.0f" r="%.0f"/></clipPath></defs>`, center, center, radius)
	fmt.Fprintf(&b, `<g clip-path="url(#c)">`)

	// Background circle.
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="%s"/>`, size, size, bg)

	// Use hash byte to pick a pattern style: rings or mosaic.
	if hash[7]%2 == 0 {
		renderRings(hash, &b, s, center, fg1, fg2)
	} else {
		renderMosaic(hash, &b, s, fg1, fg2)
	}

	b.WriteString(`</g></svg>`)
	return []byte(b.String())
}

// renderRings draws offset circles with padding from the edge.
func renderRings(hash [32]byte, b *strings.Builder, s, center float64, fg1, fg2 string) {
	padding := s * 0.08 // keep circles away from the clip edge
	maxR := s/2 - padding

	// Large background shape offset from center.
	r1 := s*0.30 + float64(hash[10])/255.0*s*0.10
	ox1 := clampOffset(hash[8], s, r1, maxR)
	oy1 := clampOffset(hash[9], s, r1, maxR)
	fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" opacity="0.7"/>`,
		center+ox1, center+oy1, r1, fg1)

	// Medium shape.
	r2 := s*0.18 + float64(hash[13])/255.0*s*0.10
	ox2 := clampOffset(hash[11], s, r2, maxR)
	oy2 := clampOffset(hash[12], s, r2, maxR)
	fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" opacity="0.8"/>`,
		center+ox2, center+oy2, r2, fg2)

	// Small accent.
	r3 := s*0.08 + float64(hash[16])/255.0*s*0.08
	ox3 := clampOffset(hash[14], s, r3, maxR)
	oy3 := clampOffset(hash[15], s, r3, maxR)
	fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" opacity="0.9"/>`,
		center+ox3, center+oy3, r3, fg1)
}

// clampOffset returns a center offset that keeps a circle of radius r within maxR from center.
func clampOffset(hashByte byte, s, r, maxR float64) float64 {
	raw := (float64(hashByte)/255.0 - 0.5) * s * 0.4
	limit := maxR - r
	if limit < 0 {
		return 0
	}
	if raw > limit {
		return limit
	}
	if raw < -limit {
		return -limit
	}
	return raw
}

// renderMosaic draws a radial grid of cells, keeping only those within the circle.
func renderMosaic(hash [32]byte, b *strings.Builder, s float64, fg1, fg2 string) {
	// 4x4 grid with mirror symmetry, centered in the circle.
	gridSize := 4
	cellSize := s / float64(gridSize+1) // leave half-cell margin
	offset := cellSize * 0.5
	center := s / 2
	radius := s * 0.48

	for row := 0; row < gridSize; row++ {
		for col := 0; col < (gridSize+1)/2; col++ {
			byteIdx := 8 + row*2 + col
			if hash[byteIdx]%3 == 0 {
				continue // skip ~1/3 of cells for visual interest
			}

			// Draw cell and its mirror.
			for _, c := range []int{col, gridSize - 1 - col} {
				cx := offset + float64(c)*cellSize + cellSize/2
				cy := offset + float64(row)*cellSize + cellSize/2
				dist := math.Sqrt((cx-center)*(cx-center) + (cy-center)*(cy-center))
				if dist+cellSize*0.4 > radius {
					continue // skip cells that would extend past the circle
				}

				color := fg1
				if hash[byteIdx]%2 == 0 {
					color = fg2
				}

				gap := cellSize * 0.08
				fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" rx="%.1f"/>`,
					offset+float64(c)*cellSize+gap, offset+float64(row)*cellSize+gap,
					cellSize-gap*2, cellSize-gap*2, color, cellSize*0.2)
			}
		}
	}
}


func hslToHex(h, s, l float64) string {
	s /= 100.0
	l /= 100.0

	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60.0, 2)-1))
	m := l - c/2

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}

	ri := int(math.Round((r + m) * 255))
	gi := int(math.Round((g + m) * 255))
	bi := int(math.Round((b + m) * 255))
	return fmt.Sprintf("#%02x%02x%02x", ri, gi, bi)
}
