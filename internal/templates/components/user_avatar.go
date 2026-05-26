//go:build !headless && !lite

package components

import (
	"net/url"
	"strings"
)

// UserAvatarSize controls the avatar's outer w/h and the inner glyph
// (letter fallback / bot tile icon) sizing. Seven sizes cover every
// surface that renders a person/agent identity in the admin UI; pick
// the one that matches the call site, not the largest that "fits".
//
//	UserAvatarXS  — 16px, inline inside text (TimelineActorInline).
//	UserAvatarSm  — 20px, list-row + small overflow badges.
//	UserAvatarMd  — 24px, timeline comment rail tile, cmdk rows.
//	UserAvatarLg  — 32px, sidebar profile.
//	UserAvatarXL  — 40px, settings style picker + user-form edit.
//	UserAvatar2XL — 48px, household member card.
//	UserAvatar3XL — 56px, user-form create + large detail headers.
type UserAvatarSize string

const (
	UserAvatarXS  UserAvatarSize = "xs"
	UserAvatarSm  UserAvatarSize = "sm"
	UserAvatarMd  UserAvatarSize = "md"
	UserAvatarLg  UserAvatarSize = "lg"
	UserAvatarXL  UserAvatarSize = "xl"
	UserAvatar2XL UserAvatarSize = "2xl"
	UserAvatar3XL UserAvatarSize = "3xl"
)

// UserAvatarProps is the prop bag for the UserAvatar component. Empty
// fields collapse cleanly: no ID + no IsAgent renders the letter
// fallback; empty Version skips the ?v= cache-buster; empty Class
// keeps just the size + shape classes.
type UserAvatarProps struct {
	// ID is the users.id / short_id used to fetch the DiceBear-backed
	// /avatars/{id} SVG. Empty falls through to the letter or bot
	// fallback (depending on IsAgent).
	ID string
	// Name drives the tooltip (`title`), alt text, and the letter
	// fallback glyph (first A–Z / 0–9 char). Required for accessibility
	// when ID is set; "?" is rendered when both are missing.
	Name string
	// Version is the cache-bust suffix appended to the URL as `?v=`.
	// Pass the user's updated_at unix timestamp; empty skips the param.
	Version string
	// Size selects one of the size variants. Empty + non-empty Class
	// lets the caller provide bespoke sizing (bb-tx-owner-badge is
	// the canonical exception).
	Size UserAvatarSize
	// IsAgent flags the actor as an AI agent. When ID is set the URL
	// is routed via ?type=agent so the configured agent DiceBear
	// style is used. When ID is empty, IsAgent triggers the bot-tile
	// fallback (lucide "bot" icon over a primary tint).
	IsAgent bool
	// Ring adds the `ring-4 ring-base-100` border that lets the avatar
	// thread through the timeline rail. Defaults off.
	Ring bool
	// Inline switches the wrapper from `flex` to `inline-block` /
	// `inline-flex` so the avatar can sit on a baseline inside a text
	// run (TimelineActorInline). Adds `align-text-bottom`.
	Inline bool
	// Class is appended to the outer wrapper. Used for one-off
	// positioning (e.g. "bb-tx-owner-badge" — which carries its own
	// w/h, rounded-full, absolute, and shadow) and for `shrink-0`.
	Class string
	// Title overrides the tooltip text. Defaults to Name when empty.
	Title string
	// SrcOverride bypasses ID-based URL construction. Used by sandbox
	// previews and any caller that already has a fully-resolved URL
	// (e.g. the settings preview proxy path). When empty, the URL is
	// built from ID + Version + IsAgent.
	SrcOverride string
	// Decorative marks the avatar as purely decorative — the
	// surrounding context already names the actor (typical inside
	// timeline rows). Renders `alt=""` so assistive tech doesn't
	// double-announce the name. Defaults off; new call sites should
	// set this when the next sibling is a `<strong>actor name</strong>`.
	Decorative bool
	// Lazy enables native browser lazy-loading on the inner <img>.
	// Use for below-the-fold avatars (long member lists, deep
	// comment threads). Skip for above-the-fold renders so the user
	// doesn't watch them pop in.
	Lazy bool
	// Loading renders a skeleton circle in place of the image.
	// Useful for hydration flicker on optimistic inserts and for
	// async-loaded actor rows. Wins over all other render branches.
	Loading bool
}

// AvatarURL is the single source of truth for a user-side avatar URL.
// id is the users.id (or short_id); version is the session's avatar_v
// cache-buster (typically the user's updated_at unix timestamp).
// Empty id returns the unknown-avatar fallback.
//
// For agent avatars, call AgentAvatarURL — it appends ?type=agent so
// the server picks the configured agent DiceBear style. For sized
// renders, call AvatarURLWith — it threads the size + actor type
// through in one shot.
//
// Mirrors the lowercased `avatarURL` helper above so templ-generated
// code (which calls the bare identifier) and external Go callers
// share a definition.
func AvatarURL(id, version string) string {
	return AvatarURLWith(id, version, "user", 0)
}

// AgentAvatarURL is AvatarURL for AI agents — appends ?type=agent so
// the server resolves the configured agent style instead of the
// user style. Pair with the agent's ID + version the same way.
func AgentAvatarURL(id, version string) string {
	return AvatarURLWith(id, version, "agent", 0)
}

// AvatarURLWith builds an avatar URL with explicit actor type +
// pixel size. Empty actor → user. Size 0 → omit the `?size=` param
// (server falls back to its 256px default). Most callers should use
// AvatarURL / AgentAvatarURL; this is the single-knob version used
// by the component when threading the size enum through.
func AvatarURLWith(id, version, actor string, size int) string {
	if id == "" {
		return "/avatars/unknown"
	}
	u := "/avatars/" + id
	params := url.Values{}
	if version != "" {
		params.Set("v", version)
	}
	if actor == "agent" {
		params.Set("type", "agent")
	}
	if size > 0 {
		params.Set("size", itoaPositive(size))
	}
	if len(params) == 0 {
		return u
	}
	return u + "?" + params.Encode()
}

// itoaPositive is a tiny strconv-free integer formatter that handles
// the positive-only avatar size values without pulling strconv into
// the templ generated code. Sizes are bounded at the call site
// (UserAvatarSize → 16..56) so we don't need the full conversion.
func itoaPositive(n int) string {
	if n <= 0 {
		return "0"
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	var buf [4]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// userAvatarTitle returns the tooltip text for a given UserAvatarProps,
// preferring an explicit Title override, then Name, then empty.
func userAvatarTitle(p UserAvatarProps) string {
	if p.Title != "" {
		return p.Title
	}
	return p.Name
}

// userAvatarAlt returns the alt text. When the avatar is marked
// decorative (the surrounding context already names the actor) we
// emit alt="" so screen readers don't double-announce; otherwise
// "<Name> avatar". Letter / bot fallbacks always have alt="" — the
// inner span carries its own aria-hidden treatment.
func userAvatarAlt(p UserAvatarProps) string {
	if p.Decorative || p.Name == "" {
		return ""
	}
	return p.Name + " avatar"
}

// userAvatarSrc returns the image src for the props, applying the
// SrcOverride escape hatch first, then routing by actor type.
func userAvatarSrc(p UserAvatarProps) string {
	if p.SrcOverride != "" {
		return p.SrcOverride
	}
	actor := "user"
	if p.IsAgent {
		actor = "agent"
	}
	return AvatarURLWith(p.ID, p.Version, actor, userAvatarPixelSize(p.Size))
}

// userAvatarLoadingAttr returns "lazy" when the prop opts in,
// otherwise "". Used as the `loading` attribute on the inner <img>.
func userAvatarLoadingAttr(p UserAvatarProps) string {
	if p.Lazy {
		return "lazy"
	}
	return ""
}

// userAvatarPixelSize maps a Size variant to its rendered pixel
// dimension. Threaded through to ?size= so the backend fetches a
// pre-sized SVG. Returns 0 when no size is set so callers fall back
// to the server's 256px default.
func userAvatarPixelSize(s UserAvatarSize) int {
	switch s {
	case UserAvatarXS:
		return 16
	case UserAvatarSm:
		return 20
	case UserAvatarMd:
		return 24
	case UserAvatarLg:
		return 32
	case UserAvatarXL:
		return 40
	case UserAvatar2XL:
		return 48
	case UserAvatar3XL:
		return 56
	}
	return 0
}

// userAvatarHasImage reports whether the component should render an
// <img> path versus a fallback (letter or bot tile). When SrcOverride
// is set we always render an image; otherwise only when ID is set.
func userAvatarHasImage(p UserAvatarProps) bool {
	if p.IsAgent {
		return false
	}
	return p.SrcOverride != "" || p.ID != ""
}

// userAvatarWrapperClass composes the outer wrapper classes for a
// UserAvatar render. Inline + Ring + Class + Size all contribute. When
// Size is empty (caller-supplied sizing via Class) we still emit a
// `rounded-full` so callers don't have to repeat it.
func userAvatarWrapperClass(p UserAvatarProps) string {
	parts := []string{}
	switch {
	case p.Inline:
		parts = append(parts, "inline-flex items-center justify-center align-text-bottom shrink-0")
	default:
		parts = append(parts, "inline-flex items-center justify-center shrink-0")
	}
	parts = append(parts, "rounded-full overflow-hidden")
	if size := userAvatarSizeClass(p.Size); size != "" {
		parts = append(parts, size)
	}
	if p.Ring {
		parts = append(parts, "ring-4 ring-base-100")
	}
	if p.Class != "" {
		parts = append(parts, p.Class)
	}
	return joinClasses(parts)
}

// userAvatarSizeClass returns the w/h tailwind utility pair for a
// given Size value, or "" when Size is empty (caller-controlled).
func userAvatarSizeClass(s UserAvatarSize) string {
	switch s {
	case UserAvatarXS:
		return "w-4 h-4"
	case UserAvatarSm:
		return "w-5 h-5"
	case UserAvatarMd:
		return "w-6 h-6"
	case UserAvatarLg:
		return "w-8 h-8"
	case UserAvatarXL:
		return "w-10 h-10"
	case UserAvatar2XL:
		return "w-12 h-12"
	case UserAvatar3XL:
		return "w-14 h-14"
	}
	return ""
}

// userAvatarImgClass returns the inner <img>'s classes. The img fills
// the wrapper (`w-full h-full object-cover`); the wrapper handles
// shape + ring. A subtle border keeps photos from blending into the
// page surface.
func userAvatarImgClass(p UserAvatarProps) string {
	return "w-full h-full object-cover bg-base-200"
}

// userAvatarFallbackClass returns the classes for the letter-fallback
// wrapper. Daisy `primary/10` tint + `text-primary/80` glyph mirrors
// the legacy `.bb-tx-owner-badge--letter` look at the larger sizes.
func userAvatarFallbackClass(p UserAvatarProps) string {
	return "w-full h-full flex items-center justify-center bg-base-200 text-base-content/60"
}

// userAvatarAgentClass returns the classes for the bot-tile fallback.
// `bg-primary/10` + `text-primary` matches the legacy timeline agent
// avatar treatment.
func userAvatarAgentClass(p UserAvatarProps) string {
	return "w-full h-full flex items-center justify-center bg-primary/10 text-primary"
}

// userAvatarLetterClass returns the size-aware text classes for the
// letter fallback glyph. Smaller sizes need tighter font + leading so
// the character fits without overflowing the tile. Two-letter renders
// (md+) trim the font by one step so "AC" doesn't visually crowd the
// tile vs a single "A".
func userAvatarLetterClass(s UserAvatarSize, twoLetters bool) string {
	switch s {
	case UserAvatarXS:
		return "text-[0.5rem] font-semibold leading-none"
	case UserAvatarSm:
		return "text-[0.625rem] font-semibold leading-none"
	case UserAvatarMd:
		if twoLetters {
			return "text-[8px] font-semibold leading-none tracking-tighter"
		}
		return "text-[10px] font-semibold leading-none"
	case UserAvatarLg:
		if twoLetters {
			return "text-[11px] font-semibold leading-none tracking-tight"
		}
		return "text-xs font-semibold leading-none"
	case UserAvatarXL:
		if twoLetters {
			return "text-[13px] font-semibold leading-none tracking-tight"
		}
		return "text-sm font-semibold leading-none"
	case UserAvatar2XL:
		if twoLetters {
			return "text-sm font-semibold leading-none"
		}
		return "text-base font-semibold leading-none"
	case UserAvatar3XL:
		if twoLetters {
			return "text-base font-semibold leading-none"
		}
		return "text-lg font-semibold leading-none"
	}
	return "text-xs font-semibold leading-none"
}

// userAvatarLetters returns the glyph(s) for a letter-fallback render.
// Mirrors the GitHub/Gravatar-style "two initials when available"
// convention used by most identity chips:
//
//	"Alice Canales"          → "AC"
//	"alice"                  → "A"
//	"Alice von Canales"      → "AC"  (first + last word's initial)
//	"AC"                     → "AC"
//	""                       → "?"
//
// For the smallest sizes (xs / sm), two letters would visually crowd
// the 16–20px tile, so we collapse back to a single initial. Sizes
// md+ get the full two-letter treatment.
func userAvatarLetters(name string, size UserAvatarSize) string {
	first := firstChar(name)
	if first == "?" {
		return "?"
	}
	if size == UserAvatarXS || size == UserAvatarSm {
		return first
	}
	words := strings.Fields(name)
	if len(words) >= 2 {
		last := firstChar(words[len(words)-1])
		if last != "?" && last != first {
			return first + last
		}
	}
	return first
}

// userAvatarTwoLetters reports whether the letter fallback for a
// given (name, size) will render two characters. Mirrors the
// userAvatarLetters branching so the font-size helper can pick the
// crowded-two-letters variant without re-parsing the name.
func userAvatarTwoLetters(name string, size UserAvatarSize) bool {
	return len(userAvatarLetters(name, size)) > 1
}

// userAvatarBotIconClass returns the size-aware lucide icon size for
// the bot fallback tile. Matches the surrounding tile width so the
// glyph reads as a filled icon, not a tiny mark on a large field.
func userAvatarBotIconClass(s UserAvatarSize) string {
	switch s {
	case UserAvatarXS:
		return "w-2.5 h-2.5"
	case UserAvatarSm:
		return "w-3 h-3"
	case UserAvatarMd:
		return "w-3 h-3"
	case UserAvatarLg:
		return "w-4 h-4"
	case UserAvatarXL:
		return "w-5 h-5"
	case UserAvatar2XL:
		return "w-6 h-6"
	case UserAvatar3XL:
		return "w-7 h-7"
	}
	return "w-4 h-4"
}

// joinClasses concatenates a list of class strings with single spaces,
// dropping empty entries so the rendered class attribute stays tight.
func joinClasses(parts []string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += " "
		}
		out += p
	}
	return out
}
