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

const monthDayFormatter = new Intl.DateTimeFormat("en-US", {
  month: "short",
  day: "numeric",
});

// formatDate renders a YYYY-MM-DD date as e.g. "Jan 2"; returns the input
// unchanged if it isn't a parseable date.
export function formatDate(date: string): string {
  const d = new Date(`${date}T00:00:00`);
  return Number.isNaN(d.getTime()) ? date : monthDayFormatter.format(d);
}

const longDateFormatter = new Intl.DateTimeFormat("en-US", {
  dateStyle: "medium",
});

// formatLongDate renders a YYYY-MM-DD date as e.g. "Jan 2, 2026".
export function formatLongDate(date: string): string {
  const d = new Date(`${date}T00:00:00`);
  return Number.isNaN(d.getTime()) ? date : longDateFormatter.format(d);
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
