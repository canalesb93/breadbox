package components

import (
	"hash/fnv"
	"strconv"
	"strings"
	"unicode"
)

// EntityAvatar is the shared rounded-square identity tile that brings the
// Workflows-gallery avatar DNA to entity surfaces (/counterparties, /recurring).
// It renders one of three shapes, picked by which fields are set, in priority
// order:
//
//  1. Image — when ImageURL is set (a counterparty's logo_url): the image,
//     object-contain on a quiet base-200 tile.
//  2. Tinted icon — when Icon is set (a series, keyed by type): a lucide glyph
//     on a semantic tone tint (the same .bb-icon-tile--* tones EntityHeader and
//     IconTile use), so a subscription/bill/loan/other reads as a distinct,
//     calm color.
//  3. Gradient monogram — the fallback (a counterparty with no logo): the first
//     one or two letters of Name on a deterministic OKLCH gradient whose hue is
//     a stable hash of Seed (or Name). This is the "colored monogram avatar for
//     custom items" half of the Workflows DNA — every nameless counterparty gets
//     a stable, legible identity color without a logo.
//
// The component is entity-neutral: both pages (and, optionally, transaction
// rows) drive it purely through props.

// EntityAvatarSize is the tile size token. Detail headers pass Size="" and size
// the tile through Class (e.g. "w-full h-full") so the avatar fills the
// EntityHeader tile slot.
type EntityAvatarSize string

const (
	// EntityAvatarSizeSM is the dense list-row tile (36×36).
	EntityAvatarSizeSM EntityAvatarSize = "sm"
	// EntityAvatarSizeMD is the default tile (40×40), matching the legacy
	// per-row logo tile the avatar replaces.
	EntityAvatarSizeMD EntityAvatarSize = "md"
)

// EntityAvatarProps configures EntityAvatar. See the component doc for the
// three render shapes and their precedence.
type EntityAvatarProps struct {
	// Name drives the monogram letters and, when Seed is empty, the monogram's
	// deterministic gradient hue. Also used as the image alt text.
	Name string
	// ImageURL, when set, renders the image shape (a counterparty logo).
	ImageURL string
	// Icon, when set (and ImageURL is empty), renders the tinted-icon shape — a
	// lucide glyph name (a series type glyph).
	Icon string
	// Tone tints the icon shape. Reuses the EntityHeader IconTone vocabulary
	// (primary/info/success/warning/error/neutral/accent). Ignored unless Icon
	// is set. Empty defaults to neutral.
	Tone IconTone
	// Seed overrides the monogram gradient hash input. Defaults to Name. Pass a
	// stable id (e.g. a short_id) when two entities legitimately share a name
	// but should read as distinct.
	Seed string
	// Size is the tile size token (sm | md). Empty lets Class drive sizing — the
	// detail-header path passes "w-full h-full" to fill the EntityHeader slot.
	Size EntityAvatarSize
	// Class appends extra utilities (sizing for the header fill, an outer ring,
	// etc.).
	Class string
}

// entityAvatarSeed returns the gradient hash input — Seed when set, else Name.
func entityAvatarSeed(p EntityAvatarProps) string {
	if strings.TrimSpace(p.Seed) != "" {
		return p.Seed
	}
	return p.Name
}

// entityAvatarHue maps a seed string to a stable hue in [0,360). FNV-1a keeps it
// deterministic and well-spread, so two different names rarely collide on color.
func entityAvatarHue(seed string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(seed))))
	return int(h.Sum32() % 360)
}

// entityAvatarHueStyle is the inline style that pins the monogram gradient's
// hue. The .bb-entity-avatar--mono class reads --bb-avatar-hue to build the
// OKLCH gradient — mirrors how .bb-tx-avatar consumes an inline --avatar-color.
func entityAvatarHueStyle(seed string) string {
	return "--bb-avatar-hue:" + strconv.Itoa(entityAvatarHue(seed))
}

// entityAvatarLetters returns the 1–2 letter monogram for a name: the initials
// of the first two words, else the first two letters of a single word, upper-
// cased. Falls back to "?" for an empty/symbol-only name.
func entityAvatarLetters(name string) string {
	fields := strings.Fields(name)
	var letters []rune
	for _, f := range fields {
		for _, r := range f {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				letters = append(letters, unicode.ToUpper(r))
				break
			}
		}
		if len(letters) == 2 {
			break
		}
	}
	if len(letters) == 0 {
		// Single word with no word-boundary second initial, or a name the field
		// split didn't yield letters for — take the first two alphanumerics.
		for _, r := range name {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				letters = append(letters, unicode.ToUpper(r))
			}
			if len(letters) == 2 {
				break
			}
		}
	} else if len(letters) == 1 {
		// One word → pad with its second alphanumeric for a fuller monogram.
		runes := []rune(strings.ToUpper(fields[0]))
		for i := 1; i < len(runes); i++ {
			if unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) {
				letters = append(letters, runes[i])
				break
			}
		}
	}
	if len(letters) == 0 {
		return "?"
	}
	return string(letters)
}

// entityAvatarTileClass assembles the tile's structural classes (flex centering,
// rounding, shrink, size token, caller extras). The shape-specific background is
// added by the templ.
func entityAvatarTileClass(size EntityAvatarSize, extra string) string {
	parts := []string{"flex items-center justify-center shrink-0 overflow-hidden", entityAvatarRadius(size)}
	if s := entityAvatarSizeClass(size); s != "" {
		parts = append(parts, s)
	}
	if strings.TrimSpace(extra) != "" {
		parts = append(parts, extra)
	}
	return strings.Join(parts, " ")
}

// entityAvatarRadius keeps the rounded-square family: rounded-lg on the dense sm
// tile, rounded-xl elsewhere (matches .bb-tx-avatar / .bb-icon-header__tile).
func entityAvatarRadius(size EntityAvatarSize) string {
	if size == EntityAvatarSizeSM {
		return "rounded-lg"
	}
	return "rounded-xl"
}

// entityAvatarSizeClass maps a size token to w/h utilities. Empty (header path)
// returns "" so the caller's Class drives sizing.
func entityAvatarSizeClass(size EntityAvatarSize) string {
	switch size {
	case EntityAvatarSizeSM:
		return "w-9 h-9"
	case EntityAvatarSizeMD:
		return "w-10 h-10"
	default:
		return ""
	}
}

// entityAvatarIconClass sizes the lucide glyph for the tinted-icon shape.
func entityAvatarIconClass(size EntityAvatarSize) string {
	switch size {
	case EntityAvatarSizeSM:
		return "w-4 h-4"
	case EntityAvatarSizeMD:
		return "w-5 h-5"
	default:
		return "w-5 h-5 sm:w-6 sm:h-6"
	}
}

// entityAvatarLetterClass sizes the monogram letters per tile size. Two letters
// get a slightly smaller type so they don't crowd the tile.
func entityAvatarLetterClass(size EntityAvatarSize, twoLetters bool) string {
	base := "font-semibold leading-none tracking-tight select-none"
	switch size {
	case EntityAvatarSizeSM:
		if twoLetters {
			return base + " text-[0.65rem]"
		}
		return base + " text-xs"
	case EntityAvatarSizeMD:
		if twoLetters {
			return base + " text-xs"
		}
		return base + " text-sm"
	default: // header fill
		if twoLetters {
			return base + " text-sm sm:text-base"
		}
		return base + " text-base sm:text-lg"
	}
}

// entityAvatarToneClass resolves the icon-shape tint to a .bb-icon-tile--* class,
// reusing the exact tones EntityHeader/IconTile use so series tiles match the
// rest of the surface. Empty tone → neutral.
func entityAvatarToneClass(t IconTone) string {
	return iconToneClass(t)
}
