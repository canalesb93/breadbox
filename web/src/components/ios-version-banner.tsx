import { useEffect, useState } from "react";
import { X } from "lucide-react";

import { cn } from "@/lib/utils";

/**
 * IOSVersionBanner — one-liner advisory shown only on iOS Safari builds
 * older than 26.2. Modern iOS and every non-iOS browser see nothing.
 *
 * Why this isn't a shadcn `<Alert>`: it's a route-level chrome element with
 * its own dismiss + persistence wiring (localStorage permanent + sessionStorage
 * per-load). Wrapping `<Alert>` would buy us inconsistent padding for the
 * single line of copy and force in-component className overrides anyway.
 * Kept as a small inline component owned by `__root.tsx`.
 */

const LOCAL_STORAGE_KEY = "breadbox:mobile-version-banner:dismissed-v1";
const SESSION_STORAGE_KEY = "breadbox:mobile-version-banner:seen-v1";

const MIN_MAJOR = 26;
const MIN_MINOR = 2;

/**
 * Result of UA parsing:
 *   `outdated`  — confirmed iOS Safari older than 26.2 → show banner
 *   `current`   — iOS Safari at or above 26.2 → hide
 *   `notApplicable` — non-iOS, in-app browser (Chrome/Edge/Firefox iOS), or
 *                     we couldn't extract a version. Hide; never show
 *                     speculatively.
 */
type Detection = "outdated" | "current" | "notApplicable";

/**
 * Detect whether the current UA is an iOS Safari build older than 26.2.
 *
 * Heuristic (intentionally conservative — false negatives preferred over
 * false positives):
 *
 *  1. UA must mention iPhone / iPad / iPod.
 *  2. UA must look like Safari proper — `Safari/<n>` present and no marker
 *     of a non-Safari iOS browser (`CriOS` = Chrome iOS, `FxiOS` = Firefox
 *     iOS, `EdgiOS` = Edge iOS). Those embed WebKit but the user can't
 *     "update iOS Safari" to fix them.
 *  3. Pull the iOS Safari version from `Version/<major>.<minor>(.<patch>)?`.
 *     If the match is missing or fails to parse, return `notApplicable` —
 *     never show the banner on uncertain UA strings.
 *  4. Compare against 26.2: major < 26, or major === 26 && minor < 2.
 *
 * Wrapped in try/catch at the caller so a regex/property failure can never
 * throw from render.
 */
export function detectIOSSafariVersion(userAgent: string): Detection {
  if (!/iPhone|iPad|iPod/.test(userAgent)) return "notApplicable";

  // In-app / non-Safari iOS browsers. Users can't fix these by updating iOS.
  if (/CriOS|FxiOS|EdgiOS|OPiOS|YaBrowser/.test(userAgent)) {
    return "notApplicable";
  }

  // Real Safari sets `Version/X.Y` in the UA; WebViews often don't.
  const versionMatch = /Version\/(\d+)\.(\d+)(?:\.(\d+))?/.exec(userAgent);
  if (!versionMatch) return "notApplicable";

  const major = Number.parseInt(versionMatch[1] ?? "", 10);
  const minor = Number.parseInt(versionMatch[2] ?? "", 10);
  if (!Number.isFinite(major) || !Number.isFinite(minor)) {
    return "notApplicable";
  }

  if (major < MIN_MAJOR) return "outdated";
  if (major === MIN_MAJOR && minor < MIN_MINOR) return "outdated";
  return "current";
}

function shouldShowBanner(): boolean {
  try {
    // Permanent dismissal beats everything.
    if (window.localStorage.getItem(LOCAL_STORAGE_KEY) === "1") return false;
    // Once-per-tab gating: if we already showed the banner in this tab
    // (set when the banner first mounts in useEffect below), don't show
    // it again — a soft reload in the same tab shouldn't re-pop it. New
    // tabs get a fresh sessionStorage scope so the message is still
    // discoverable.
    if (window.sessionStorage.getItem(SESSION_STORAGE_KEY) === "1") return false;
    const result = detectIOSSafariVersion(window.navigator.userAgent);
    return result === "outdated";
  } catch {
    return false;
  }
}

export function IOSVersionBanner() {
  const [visible, setVisible] = useState<boolean>(() => {
    if (typeof window === "undefined") return false;
    return shouldShowBanner();
  });

  useEffect(() => {
    if (!visible) return;
    try {
      // Mark as seen-this-load. If the user dismisses, we also write the
      // permanent localStorage flag below so future tabs stay quiet too.
      window.sessionStorage.setItem(SESSION_STORAGE_KEY, "1");
    } catch {
      // Private mode or storage disabled — banner still renders; just won't
      // remember across reloads. Acceptable degradation.
    }
  }, [visible]);

  if (!visible) return null;

  const dismiss = () => {
    setVisible(false);
    try {
      window.localStorage.setItem(LOCAL_STORAGE_KEY, "1");
    } catch {
      // Same fallback as above — best effort.
    }
  };

  return (
    <div
      role="status"
      aria-live="polite"
      className={cn(
        "bg-muted text-muted-foreground border-border flex items-center gap-2 rounded-md border px-3 py-2 text-xs sm:text-sm",
      )}
    >
      <span className="flex-1">
        For the best experience, update to iOS 26.2 or later.
      </span>
      <button
        type="button"
        onClick={dismiss}
        aria-label="Dismiss iOS update notice"
        className={cn(
          "text-muted-foreground hover:text-foreground focus-visible:ring-ring relative -mr-1 inline-flex size-7 shrink-0 items-center justify-center rounded-md transition-colors focus-visible:ring-2 focus-visible:outline-none",
          // 44pt tap target on touch devices — matches the tag-chip × recipe
          // documented in .claude/rules/v2-frontend.md (Tap targets).
          "pointer-coarse:before:absolute pointer-coarse:before:top-1/2 pointer-coarse:before:left-1/2 pointer-coarse:before:size-11 pointer-coarse:before:-translate-x-1/2 pointer-coarse:before:-translate-y-1/2 pointer-coarse:before:content-['']",
        )}
      >
        <X className="size-3.5" aria-hidden />
      </button>
    </div>
  );
}
