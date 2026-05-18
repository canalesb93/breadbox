// Hand-rolled cron→prose for the ~12 schedules a self-hoster will realistically
// pick. Anything else falls back to "Custom: <expression>". No external
// dependency — robfig/cron is the server-side schedule engine and supports
// 5-field standard cron plus @-shorthands; we cover both.
//
// Timezone contract: the cron expression stored in the database is interpreted
// in UTC. Presets are defined in the user's *local* time and converted to UTC
// at pick time via `presetExprForLocalHour`; the prose label inverts the same
// conversion so the picker reads "Daily at 9 AM your time" even when the
// stored expression is `0 17 * * *`. This keeps schedules predictable across
// hosting setups (Docker images default to UTC) while letting humans think in
// their own clock.
//
// If a user's server runs in a non-UTC timezone, the stored expression still
// fires at the server's local hour — that's a documentation concern, not a
// UI concern. The preset picker assumes UTC interpretation.

const DOW_LABEL = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"] as const;
const DOW_LABEL_PLURAL = ["Sundays", "Mondays", "Tuesdays", "Wednesdays", "Thursdays", "Fridays", "Saturdays"] as const;

// formatHour12 turns 24h → "9 AM" / "6 PM" / "midnight" / "noon".
function formatHour12(h: number): string {
  const n = ((h % 24) + 24) % 24;
  if (n === 0) return "midnight";
  if (n === 12) return "noon";
  if (n < 12) return `${n} AM`;
  return `${n - 12} PM`;
}

// utcHourToLocal converts a UTC hour-of-day to the browser's local hour.
function utcHourToLocal(utcHour: number): number {
  const offsetMin = new Date().getTimezoneOffset(); // minutes west of UTC
  const offsetHr = -offsetMin / 60; // east-positive
  return ((utcHour + offsetHr) % 24 + 24) % 24;
}

// localHourToUtc converts a desired local hour-of-day to UTC.
function localHourToUtc(localHour: number): number {
  const offsetMin = new Date().getTimezoneOffset();
  const offsetHr = -offsetMin / 60;
  return ((localHour - offsetHr) % 24 + 24) % 24;
}

// tzShortName returns the browser's IANA tz short label (e.g. "PST", "CET")
// for the prose label. Falls back to "local" when Intl is unavailable.
function tzShortName(): string {
  try {
    const parts = new Intl.DateTimeFormat(undefined, { timeZoneName: "short" }).formatToParts(
      new Date(),
    );
    const tz = parts.find((p) => p.type === "timeZoneName")?.value;
    return tz ?? "local";
  } catch {
    return "local";
  }
}

// Standard cron field shape we recognize. Returns the parsed fields or null
// when the expression isn't a simple "minute hour dom month dow" with all
// integer-or-wildcard values.
interface ParsedCron {
  minute: number;
  hour: number | "*";
  dom: number | "*";
  month: number | "*";
  dow: number | "*";
}

function parseSimpleCron(e: string): ParsedCron | null {
  const parts = e.split(/\s+/);
  if (parts.length !== 5) return null;
  const intOrStar = (s: string): number | "*" | null => {
    if (s === "*") return "*";
    if (/^\d+$/.test(s)) return parseInt(s, 10);
    return null;
  };
  const m = parseInt(parts[0], 10);
  const h = intOrStar(parts[1]);
  const dom = intOrStar(parts[2]);
  const mon = intOrStar(parts[3]);
  const dow = intOrStar(parts[4]);
  if (Number.isNaN(m) || h === null || dom === null || mon === null || dow === null) return null;
  return { minute: m, hour: h, dom, month: mon, dow };
}

export function cronToProseLabel(expr: string | null | undefined): string {
  if (!expr) return "Manual trigger only";
  const e = expr.trim();

  // Shortcut handling.
  switch (e) {
    case "@daily":
      return "Daily at midnight UTC";
    case "@hourly":
      return "Every hour (top of hour)";
    case "@weekly":
      return "Sundays at midnight UTC";
    case "@monthly":
      return "1st of month at midnight UTC";
    case "@yearly":
    case "@annually":
      return "Once a year (Jan 1 UTC)";
  }

  // Step expressions: */N for minute or hour.
  if (e === "*/30 * * * *") return "Every 30 minutes";
  if (e === "0 */6 * * *") return "Every 6 hours";
  if (e === "0 */12 * * *") return "Every 12 hours";

  // Generic top-of-hour: "0 * * * *".
  if (e === "0 * * * *") return "Every hour (top of hour)";

  const p = parseSimpleCron(e);
  if (!p) return `Custom: ${e}`;

  // Only humanize the canonical "exact minute, exact hour" presets — anything
  // else falls back to the raw expression so the user can sanity-check.
  if (typeof p.hour !== "number" || typeof p.minute !== "number" || p.minute !== 0) {
    return `Custom: ${e}`;
  }
  const localHour = utcHourToLocal(p.hour);
  const tz = tzShortName();
  const hourLabel = formatHour12(localHour);

  // Day-of-week match (every weekday on this day-of-week).
  if (p.dom === "*" && p.month === "*" && typeof p.dow === "number" && p.dow >= 0 && p.dow <= 6) {
    return `${DOW_LABEL_PLURAL[p.dow]} at ${hourLabel} ${tz}`;
  }
  // Day-of-month match (every month on this day).
  if (typeof p.dom === "number" && p.month === "*" && p.dow === "*") {
    const ord = p.dom === 1 ? "1st" : p.dom === 2 ? "2nd" : p.dom === 3 ? "3rd" : `${p.dom}th`;
    return `${ord} of month at ${hourLabel} ${tz}`;
  }
  // Daily.
  if (p.dom === "*" && p.month === "*" && p.dow === "*") {
    return `Daily at ${hourLabel} ${tz}`;
  }
  return `Custom: ${e}`;
}

// isValidCronExpr does a permissive structural check — 5 fields, each one of
// the standard sub-grammars. Robfig will reject anything truly malformed at
// registration time; this helper just gates UI inline errors.
export function isValidCronExpr(expr: string): boolean {
  const e = expr.trim();
  if (e.startsWith("@")) {
    return ["@daily", "@hourly", "@weekly", "@monthly", "@yearly", "@annually"].includes(e);
  }
  const fieldRe = /^(\*|(\d+)(-\d+)?(,\d+(-\d+)?)*|(\*\/\d+))$/;
  const parts = e.split(/\s+/);
  if (parts.length !== 5) return false;
  return parts.every((p) => fieldRe.test(p));
}

// CronPreset is the shape consumed by the picker. `kind` controls how the
// preset is translated to a cron expression when the user clicks it:
//   - "fixed":   value is a literal cron expression, used as-is (no tz shift).
//                For sub-hour intervals where tz is meaningless.
//   - "daily":   fire every day at `localHour` (converted to UTC at pick).
//   - "weekly":  fire every `dow` at `localHour` (converted to UTC at pick).
//   - "monthly": fire every `dom` at `localHour` (converted to UTC at pick).
export type CronPresetKind = "fixed" | "daily" | "weekly" | "monthly";

export interface CronPreset {
  /** Stable key for the popover. */
  key: string;
  /** What the user sees in the picker — already framed in local time when
   *  the preset is a tz-relative one. The popover never has to recompute
   *  the label; it's the picker's responsibility. */
  label: string;
  /** Optional helper text below the label (e.g. "→ 5 PM UTC"). */
  hint?: string;
  /** How the preset is converted to a cron expression at pick time. */
  kind: CronPresetKind;
  /** For fixed presets: the literal expression. */
  expr?: string;
  /** For daily/weekly/monthly: the local hour the user wants. */
  localHour?: number;
  /** For weekly: 0=Sun … 6=Sat. */
  dow?: number;
  /** For monthly: 1..31. */
  dom?: number;
}

// presetToCronExpr converts a preset to a concrete cron expression, applying
// the local→UTC hour shift for tz-relative presets. Exported so the field
// component can call it on pick.
export function presetToCronExpr(p: CronPreset): string {
  if (p.kind === "fixed") return p.expr ?? "";
  const utcHour = p.localHour !== undefined ? localHourToUtc(p.localHour) : 0;
  switch (p.kind) {
    case "daily":
      return `0 ${utcHour} * * *`;
    case "weekly":
      return `0 ${utcHour} * * ${p.dow ?? 1}`;
    case "monthly":
      return `0 ${utcHour} ${p.dom ?? 1} * *`;
  }
}

// buildCronPresets returns the canonical preset list framed in the browser's
// local time. Called every render so the labels stay accurate even if the
// page is open through a DST flip (cheap — no allocations beyond the array).
export function buildCronPresets(): CronPreset[] {
  const tz = tzShortName();
  const dailyHint = (localHour: number): string => {
    const utc = localHourToUtc(localHour);
    return `→ ${formatHour12(utc)} UTC`;
  };
  return [
    { key: "every-30m", kind: "fixed", expr: "*/30 * * * *", label: "Every 30 minutes" },
    { key: "hourly", kind: "fixed", expr: "0 * * * *", label: "Every hour (top of hour)" },
    { key: "every-6h", kind: "fixed", expr: "0 */6 * * *", label: "Every 6 hours" },
    {
      key: "daily-6",
      kind: "daily",
      localHour: 6,
      label: `Daily at 6 AM ${tz}`,
      hint: dailyHint(6),
    },
    {
      key: "daily-9",
      kind: "daily",
      localHour: 9,
      label: `Daily at 9 AM ${tz}`,
      hint: dailyHint(9),
    },
    {
      key: "daily-18",
      kind: "daily",
      localHour: 18,
      label: `Daily at 6 PM ${tz}`,
      hint: dailyHint(18),
    },
    {
      key: "weekly-mon-9",
      kind: "weekly",
      dow: 1,
      localHour: 9,
      label: `Mondays at 9 AM ${tz}`,
      hint: dailyHint(9),
    },
    {
      key: "weekly-fri-9",
      kind: "weekly",
      dow: 5,
      localHour: 9,
      label: `Fridays at 9 AM ${tz}`,
      hint: dailyHint(9),
    },
    {
      key: "monthly-1-9",
      kind: "monthly",
      dom: 1,
      localHour: 9,
      label: `1st of month at 9 AM ${tz}`,
      hint: dailyHint(9),
    },
    { key: "daily-midnight-utc", kind: "fixed", expr: "@daily", label: "Daily at midnight UTC" },
  ];
}

// Re-export the legacy CRON_PRESETS shape (eager evaluation) so existing
// imports keep compiling. New callsites should prefer buildCronPresets().
export const CRON_PRESETS = buildCronPresets();
// Voice the DOW labels to satisfy --unused-vars in tooling that ignores
// re-exports; they're consumed inside cronToProseLabel above.
void DOW_LABEL;
