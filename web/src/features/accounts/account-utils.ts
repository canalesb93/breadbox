import {
  Banknote,
  CreditCard,
  Landmark,
  PiggyBank,
  Wallet,
  type LucideIcon,
} from "lucide-react";
import type { Account } from "@/api/types";

// TYPE_ICON maps Plaid-ish account types to lucide icons. The same map
// powers list rows, the detail page hero, and the connection-detail
// account cards — keep it as the single source.
const TYPE_ICON: Record<string, LucideIcon> = {
  depository: PiggyBank,
  credit: CreditCard,
  loan: Landmark,
  investment: Wallet,
};

export function accountTypeIcon(type: string): LucideIcon {
  return TYPE_ICON[type] ?? Banknote;
}

// Liabilities (credit cards, loans) render the balance as money owed —
// flipped sign convention vs depository/investment.
export function isLiability(type: string): boolean {
  return type === "credit" || type === "loan";
}

// Account "label" — the bank-provided name unless the user has set a
// display_name override. Keep the precedence logic in one place so the
// list and the detail header always pick the same label.
export function accountLabel(a: Pick<Account, "name"> & { display_name?: string | null }): string {
  if (a.display_name && a.display_name.trim().length > 0) return a.display_name;
  return a.name;
}

const TYPE_LABEL: Record<string, string> = {
  depository: "Bank",
  credit: "Credit card",
  loan: "Loan",
  investment: "Investment",
  other: "Account",
};

// Capitalised type label e.g. "Bank · checking", "Credit card · personal".
// Subtypes arrive snake_cased from providers; normalise to spaces so the
// `capitalize` Tailwind class doesn't render "Credit_card".
export function accountTypeLabel(type: string, subtype: string | null): string {
  const base = TYPE_LABEL[type] ?? type;
  if (!subtype) return base;
  const sub = subtype.toLowerCase().replace(/_/g, " ");
  // Avoid double-printing when the subtype already names the type
  // ("credit card · credit card" reads worse than just "Credit card").
  if (base.toLowerCase().includes(sub) || sub.includes(base.toLowerCase())) {
    return base;
  }
  return `${base} · ${sub}`;
}

// Credit-utilisation as a 0–100 percentage. Returns null when the account
// doesn't carry a limit (or the limit is zero — defensive against bad data).
export function creditUtilization(a: Pick<Account, "balance_current" | "balance_limit">): number | null {
  if (a.balance_limit == null || a.balance_limit <= 0) return null;
  if (a.balance_current == null) return null;
  return Math.min(100, Math.max(0, (a.balance_current / a.balance_limit) * 100));
}

// Tailwind utility for the utilization bar — escalates green → amber → red
// as the bar approaches the limit.
export function utilizationBarClass(pct: number): string {
  if (pct >= 90) return "bg-destructive";
  if (pct >= 75) return "bg-amber-500";
  return "bg-emerald-500";
}

// formatCurrency renders an amount in the supplied ISO code via Intl.
// Currency keys are scarce enough that we don't bother caching formatters
// (cf. lib/format.ts) — most accounts use one of two currencies and
// list pages re-render rarely.
export function formatCurrency(amount: number, currency: string | null): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: currency ?? "USD",
      maximumFractionDigits: 2,
    }).format(amount);
  } catch {
    return `${amount.toFixed(2)} ${currency ?? ""}`.trim();
  }
}

export interface CurrencyTotal {
  currency: string;
  total: number;
  count: number;
}

// totalsByCurrency aggregates account balances across the supplied set,
// keyed by ISO currency. Sign convention matches the user's mental model:
// assets (depository, investment) are positive; liabilities (credit, loan)
// flip sign so the total reads as net worth. Dependent-linked accounts are
// excluded — they're already counted under their primary cardholder.
export function totalsByCurrency(accounts: Account[]): CurrencyTotal[] {
  const m = new Map<string, CurrencyTotal>();
  for (const a of accounts) {
    if (a.is_dependent_linked) continue;
    if (a.balance_current == null || !a.iso_currency_code) continue;
    const signed = isLiability(a.type) ? -a.balance_current : a.balance_current;
    const prev = m.get(a.iso_currency_code) ?? {
      currency: a.iso_currency_code,
      total: 0,
      count: 0,
    };
    prev.total += signed;
    prev.count += 1;
    m.set(a.iso_currency_code, prev);
  }
  // Most accounts share a single currency — present the biggest set first.
  return [...m.values()].sort((a, b) => b.count - a.count);
}

// Group accounts by a chosen dimension. We render the page grouped by
// connection (institution) by default since that's how users think about
// their money; "type" gives a depository-vs-credit overview.
export type AccountGroupBy = "institution" | "type";

export interface AccountGroup {
  key: string;
  label: string;
  accounts: Account[];
}

// groupNetTotal returns the signed net balance for a group of accounts in
// the group's dominant currency. Liabilities flip sign so the total reads
// as "what this slice contributes to net worth." Mixed-currency groups
// fall back to the most-populated currency; the count of accounts excluded
// from the total (because they're in a different currency or have no
// balance) is returned alongside so the header can hint at it.
export interface GroupTotal {
  currency: string | null;
  total: number;
  excluded: number;
}

export function groupNetTotal(accounts: Account[]): GroupTotal {
  // Pick the most-populated currency in the group as the display currency.
  const counts = new Map<string, number>();
  for (const a of accounts) {
    if (!a.iso_currency_code) continue;
    counts.set(a.iso_currency_code, (counts.get(a.iso_currency_code) ?? 0) + 1);
  }
  let primary: string | null = null;
  let max = 0;
  for (const [k, v] of counts) {
    if (v > max) {
      max = v;
      primary = k;
    }
  }
  if (primary == null) {
    return { currency: null, total: 0, excluded: accounts.length };
  }
  let total = 0;
  let excluded = 0;
  for (const a of accounts) {
    if (a.is_dependent_linked) continue;
    if (a.balance_current == null) {
      excluded += 1;
      continue;
    }
    if (a.iso_currency_code !== primary) {
      excluded += 1;
      continue;
    }
    total += isLiability(a.type) ? -a.balance_current : a.balance_current;
  }
  return { currency: primary, total, excluded };
}

export function groupAccounts(
  accounts: Account[],
  by: AccountGroupBy,
): AccountGroup[] {
  const m = new Map<string, AccountGroup>();
  for (const a of accounts) {
    let key: string;
    let label: string;
    if (by === "institution") {
      key = a.connection_id ?? "_unlinked";
      label = a.institution_name ?? "Manual / CSV";
    } else {
      key = a.type;
      label = TYPE_LABEL[a.type] ?? a.type;
    }
    let g = m.get(key);
    if (!g) {
      g = { key, label, accounts: [] };
      m.set(key, g);
    }
    g.accounts.push(a);
  }
  // Most-populated groups first; ties fall back to alphabetical label.
  return [...m.values()].sort((a, b) => {
    if (b.accounts.length !== a.accounts.length) {
      return b.accounts.length - a.accounts.length;
    }
    return a.label.localeCompare(b.label);
  });
}
