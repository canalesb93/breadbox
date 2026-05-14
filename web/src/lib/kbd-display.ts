// Shared key-label formatting for the shortcut sheet and kbd tooltips — one
// source of truth so the two surfaces never drift.

const DISPLAY: Record<string, string> = {
  mod: "⌘",
  cmd: "⌘",
  ctrl: "Ctrl",
  shift: "⇧",
  alt: "⌥",
  option: "⌥",
};

// displayKey maps a raw shortcut key to its glyph — "mod" → "⌘", single
// letters uppercased, everything else passed through.
export function displayKey(key: string): string {
  const lower = key.toLowerCase();
  if (lower in DISPLAY) return DISPLAY[lower];
  return key.length === 1 ? key.toUpperCase() : key;
}
