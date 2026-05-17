// Hand-rolled cron→prose for the ~12 schedules a self-hoster will realistically
// pick. Anything else falls back to "Custom: <expression>". No external
// dependency — robfig/cron is the server-side schedule engine and supports
// 5-field standard cron plus @-shorthands; we cover both.

export function cronToProseLabel(expr: string | null | undefined): string {
  if (!expr) return "Manual trigger only";
  const e = expr.trim();
  switch (e) {
    case "@daily":
    case "0 0 * * *":
      return "Daily at midnight";
    case "0 6 * * *":
      return "Daily at 6 AM";
    case "0 9 * * *":
      return "Daily at 9 AM";
    case "0 18 * * *":
      return "Daily at 6 PM";
    case "0 9 * * 1":
      return "Mondays at 9 AM";
    case "0 9 * * 5":
      return "Fridays at 9 AM";
    case "0 9 1 * *":
      return "1st of month at 9 AM";
    case "0 0 1 * *":
      return "1st of month at midnight";
    case "*/30 * * * *":
      return "Every 30 minutes";
    case "0 */6 * * *":
      return "Every 6 hours";
    case "@weekly":
    case "0 0 * * 0":
      return "Sundays at midnight";
    case "@hourly":
    case "0 * * * *":
      return "Every hour";
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

export interface CronPreset {
  value: string;
  label: string;
}

// The preset list shown in the CronField popover. Keep ordered by frequency
// (high → low) so users see the most likely choices first.
export const CRON_PRESETS: CronPreset[] = [
  { value: "*/30 * * * *", label: "Every 30 minutes" },
  { value: "0 * * * *", label: "Every hour (top of hour)" },
  { value: "0 */6 * * *", label: "Every 6 hours" },
  { value: "0 6 * * *", label: "Daily at 6 AM" },
  { value: "0 9 * * *", label: "Daily at 9 AM" },
  { value: "0 18 * * *", label: "Daily at 6 PM" },
  { value: "0 9 * * 1", label: "Mondays at 9 AM" },
  { value: "0 9 * * 5", label: "Fridays at 9 AM" },
  { value: "0 9 1 * *", label: "1st of month at 9 AM" },
  { value: "@daily", label: "Daily (midnight)" },
  { value: "@hourly", label: "Hourly (top of hour)" },
];
