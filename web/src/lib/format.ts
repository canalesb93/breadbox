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
