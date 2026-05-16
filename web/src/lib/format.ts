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
