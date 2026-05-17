// Shared display formatters. `Intl.*Format` construction is the expensive
// part of the API — building one per cell render janks long lists — so
// instances are cached and only `.format()` is called per value.

const currencyFormatters = new Map<string, Intl.NumberFormat>();

function currencyFormatter(currency: string): Intl.NumberFormat {
  let f = currencyFormatters.get(currency);
  if (!f) {
    f = new Intl.NumberFormat("en-US", { style: "currency", currency });
    currencyFormatters.set(currency, f);
  }
  return f;
}

// formatAmount renders a transaction amount. Sign convention: positive =
// money out (plain), negative = money in (prefixed with +).
export function formatAmount(amount: number, currency: string | null): string {
  const formatted = currencyFormatter(currency ?? "USD").format(Math.abs(amount));
  return amount < 0 ? `+${formatted}` : formatted;
}

// formatBalance renders an account balance / non-transaction amount with the
// sign preserved (no leading + for negatives). Use this for balances,
// totals, and any surface that isn't following the transaction sign
// convention. Cached `Intl.NumberFormat` per currency, same as
// `formatAmount`.
export function formatBalance(amount: number, currency: string | null): string {
  try {
    return currencyFormatter(currency ?? "USD").format(amount);
  } catch {
    return `${amount.toFixed(2)} ${currency ?? ""}`.trim();
  }
}

const compactCurrencyFormatters = new Map<string, Intl.NumberFormat>();

function compactCurrencyFormatter(currency: string): Intl.NumberFormat {
  let f = compactCurrencyFormatters.get(currency);
  if (!f) {
    f = new Intl.NumberFormat("en-US", {
      style: "currency",
      currency,
      maximumFractionDigits: 0,
    });
    compactCurrencyFormatters.set(currency, f);
  }
  return f;
}

// formatCompactAmount renders a currency amount without minor units —
// `$1,234` rather than `$1,234.56`. Used for hero KPIs where two decimals
// add noise but the headline number is what the user is scanning. Cached
// `Intl.NumberFormat` per currency.
export function formatCompactAmount(amount: number, currency: string | null): string {
  try {
    return compactCurrencyFormatter(currency ?? "USD").format(amount);
  } catch {
    return `${Math.round(amount)} ${currency ?? ""}`.trim();
  }
}

// parseIsoDate parses a YYYY-MM-DD string as a local-midnight Date so day-of
// boundaries don't drift by a day when the user's timezone is west of UTC.
// (`new Date("2026-01-02")` parses as UTC midnight, then renders as the prior
// day in negative-offset zones.)
export function parseIsoDate(iso: string): Date {
  return new Date(`${iso}T00:00:00`);
}

const monthDayFormatter = new Intl.DateTimeFormat("en-US", {
  month: "short",
  day: "numeric",
});

// formatDate renders a YYYY-MM-DD date as e.g. "Jan 2"; returns the input
// unchanged if it isn't a parseable date.
export function formatDate(date: string): string {
  const d = parseIsoDate(date);
  return Number.isNaN(d.getTime()) ? date : monthDayFormatter.format(d);
}

const longDateFormatter = new Intl.DateTimeFormat("en-US", {
  dateStyle: "medium",
});

// formatLongDate renders a YYYY-MM-DD date as e.g. "Jan 2, 2026".
export function formatLongDate(date: string): string {
  const d = parseIsoDate(date);
  return Number.isNaN(d.getTime()) ? date : longDateFormatter.format(d);
}

const dateTimeFormatter = new Intl.DateTimeFormat("en-US", {
  dateStyle: "medium",
  timeStyle: "short",
});

// formatDateTime renders an RFC3339 timestamp as e.g. "Jan 2, 2026, 3:45 PM".
// Use for "last updated" / "created at" rows where the time-of-day matters.
// Returns the input unchanged if it isn't a parseable date.
export function formatDateTime(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : dateTimeFormatter.format(d);
}

const relativeTimeFormatter = new Intl.RelativeTimeFormat("en-US", {
  numeric: "auto",
});

const RELATIVE_UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ["year", 31_536_000],
  ["month", 2_592_000],
  ["week", 604_800],
  ["day", 86_400],
  ["hour", 3_600],
  ["minute", 60],
];

// formatRelativeTime renders an RFC3339 timestamp as e.g. "3 hours ago" or
// "in 2 days"; returns the input unchanged if it isn't a parseable date.
// Long form — use in body copy and tooltips. For compact pills/badges, use
// `formatRelativeShort`.
export function formatRelativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return iso;
  const diffSec = (then - Date.now()) / 1000;
  const absSec = Math.abs(diffSec);
  for (const [unit, secs] of RELATIVE_UNITS) {
    if (absSec >= secs) {
      return relativeTimeFormatter.format(Math.round(diffSec / secs), unit);
    }
  }
  return relativeTimeFormatter.format(Math.round(diffSec), "second");
}

// formatRelativeShort renders an RFC3339 timestamp as a compact "12m ago" /
// "3h ago" / "5d ago" — the dense form for connection rows, sync history,
// and other surfaces where the long form ("12 minutes ago") would crowd
// the layout. Falls back to an ISO date past 30 days, and returns "never"
// when the input is null. Mirrors the v1 LastSyncedAtRelative shape.
export function formatRelativeShort(
  iso: string | null,
  now: Date = new Date(),
): string {
  if (!iso) return "never";
  const then = new Date(iso);
  if (Number.isNaN(then.getTime())) return iso;
  const diffSec = Math.floor((now.getTime() - then.getTime()) / 1000);
  if (diffSec < 30) return "just now";
  if (diffSec < 60) return `${diffSec}s ago`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `${diffHr}h ago`;
  const diffDay = Math.floor(diffHr / 24);
  if (diffDay < 30) return `${diffDay}d ago`;
  return then.toISOString().slice(0, 10);
}
