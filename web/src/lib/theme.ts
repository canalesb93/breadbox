// Theme presets + mode handling for the v2 SPA.
//
// A theme has two axes:
//   - `mode`: light, dark, or system (follow the OS).
//   - `preset`: a named CSS-variable block defined in globals.css that
//     overrides the accent color family (primary/ring/sidebar-primary).
//
// We apply the chosen theme by toggling a `.dark` class and a `data-theme`
// attribute on `<html>`. Every preset has a light AND dark variant so the
// two axes compose. Persistence lives in localStorage under THEME_STORAGE_KEY.
//
// The first apply runs *before* React mounts via an inline <script> in
// index.html — that script reads the same storage key. Keep the storage
// shape there in sync with this file.

export type ThemeMode = "system" | "light" | "dark";
export type ThemePreset =
  | "neutral"
  | "blue"
  | "green"
  | "rose"
  | "orange";

export interface ThemeState {
  mode: ThemeMode;
  preset: ThemePreset;
}

export const THEME_STORAGE_KEY = "breadbox-v2-theme";

export const DEFAULT_THEME: ThemeState = {
  mode: "system",
  preset: "neutral",
};

export interface ThemePresetMeta {
  id: ThemePreset;
  label: string;
  // Swatch shown in the picker — pulls from the CSS variable so swatches
  // re-color when the page itself does (e.g. light/dark mode).
  swatchVar: string;
}

export const THEME_PRESETS: ThemePresetMeta[] = [
  { id: "neutral", label: "Neutral", swatchVar: "--primary" },
  { id: "blue", label: "Blue", swatchVar: "--primary" },
  { id: "green", label: "Green", swatchVar: "--primary" },
  { id: "rose", label: "Rose", swatchVar: "--primary" },
  { id: "orange", label: "Orange", swatchVar: "--primary" },
];

export const THEME_MODES: { id: ThemeMode; label: string }[] = [
  { id: "system", label: "System" },
  { id: "light", label: "Light" },
  { id: "dark", label: "Dark" },
];

export function resolveMode(mode: ThemeMode): "light" | "dark" {
  if (mode === "system") {
    return window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  }
  return mode;
}

export function applyTheme(state: ThemeState): void {
  const root = document.documentElement;
  const resolved = resolveMode(state.mode);
  root.classList.toggle("dark", resolved === "dark");
  if (state.preset === "neutral") {
    root.removeAttribute("data-theme");
  } else {
    root.setAttribute("data-theme", state.preset);
  }
}

export function loadTheme(): ThemeState {
  try {
    const raw = localStorage.getItem(THEME_STORAGE_KEY);
    if (!raw) return DEFAULT_THEME;
    const parsed = JSON.parse(raw) as Partial<ThemeState>;
    return {
      mode: isMode(parsed.mode) ? parsed.mode : DEFAULT_THEME.mode,
      preset: isPreset(parsed.preset) ? parsed.preset : DEFAULT_THEME.preset,
    };
  } catch {
    return DEFAULT_THEME;
  }
}

export function saveTheme(state: ThemeState): void {
  try {
    localStorage.setItem(THEME_STORAGE_KEY, JSON.stringify(state));
  } catch {
    // ignore — storage may be unavailable (private mode, quota).
  }
}

function isMode(v: unknown): v is ThemeMode {
  return v === "system" || v === "light" || v === "dark";
}

function isPreset(v: unknown): v is ThemePreset {
  return (
    v === "neutral" ||
    v === "blue" ||
    v === "green" ||
    v === "rose" ||
    v === "orange"
  );
}
