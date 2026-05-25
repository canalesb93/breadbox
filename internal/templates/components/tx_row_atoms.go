//go:build !headless && !lite

package components

// txAvatarClass builds the bb-tx-avatar class string for the
// requested size and category state. Centralising the matrix here
// keeps the avatar size knob honest — every variant gets the same
// --sm suffix and the same uncategorised fallback class.
func txAvatarClass(size TxAvatarSize, uncategorized bool) string {
	base := "bb-tx-avatar"
	if size == TxAvatarSizeSm {
		base += " bb-tx-avatar--sm"
	}
	if uncategorized {
		base += " bb-tx-avatar--uncategorized"
	}
	return base
}

// txAvatarIconClass returns the Lucide icon sizing for the avatar's
// glyph. Default 36×36 avatars take w-4 h-4; --sm (32×32) takes
// w-3.5 h-3.5 so the icon's optical weight matches the smaller well.
func txAvatarIconClass(size TxAvatarSize) string {
	if size == TxAvatarSizeSm {
		return "w-3.5 h-3.5"
	}
	return "w-4 h-4"
}

// txAvatarWrapperClass returns the outer wrapper class for the avatar
// composition. The base `relative shrink-0` is always present so the
// owner-badge anchors correctly; `extra` is appended verbatim so
// callers can layer in slot-specific classes (e.g. `bb-tx-avatar-slot`
// on TxRow so mobile bulk-select can hide the slot).
func txAvatarWrapperClass(extra string) string {
	base := "relative shrink-0"
	if extra != "" {
		return base + " " + extra
	}
	return base
}
