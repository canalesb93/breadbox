import type { Account, Connection } from "@/api/types";

// Provider display label — capitalised single word.
export function providerLabel(provider: string): string {
  if (provider === "csv") return "CSV";
  return provider.charAt(0).toUpperCase() + provider.slice(1);
}

// Status badge tone — maps the canonical status enum onto a shadcn-friendly
// variant the row component can pass to <Badge>.
export type StatusTone = "active" | "warning" | "destructive" | "muted";

export function statusTone(status: string): StatusTone {
  switch (status) {
    case "active":
      return "active";
    case "pending_reauth":
      return "warning";
    case "error":
      return "destructive";
    default:
      return "muted"; // disconnected, unknown
  }
}

export function statusLabel(status: string): string {
  switch (status) {
    case "active":
      return "Active";
    case "pending_reauth":
      return "Re-auth needed";
    case "error":
      return "Error";
    case "disconnected":
      return "Disconnected";
    default:
      return status;
  }
}

// needsAttention is the grouping rule for the top banner ("N connections need
// attention") — anything that requires user action to resume syncing.
export function needsAttention(c: Connection): boolean {
  return c.status === "pending_reauth" || c.status === "error";
}

// Relative time helper — keep the dependency-free version tiny. Returns "12m
// ago", "2h ago", "3d ago", or an ISO date when older than 30 days. Mirrors
// the v1 LastSyncedAtRelative shape closely enough for parity.
export function relativeTime(iso: string | null, now: Date = new Date()): string {
  if (!iso) return "never";
  const then = new Date(iso);
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

export interface ConnectionAccountStats {
  count: number;
  // Total balance keyed by currency. Never sum across currencies — the row
  // renders the user's primary (most-common) currency and notes the rest in a
  // tooltip if needed.
  totalsByCurrency: Map<string, number>;
}

// Aggregates the global accounts list down to per-connection stats. We compute
// client-side rather than asking the API because the SPA already caches
// /api/v1/accounts for the transactions filter — joining here is free.
//
// The Map is keyed by the connection's **short_id** because that's what the
// /accounts endpoint exposes as `account.connection_id` (the project-wide
// compact-ID convention; the connection's UUID is `account.connection_id`'s
// long form, never returned here).
//
// Dependent-linked accounts are omitted from balance sums so the connection
// row matches what /accounts would show, but their count is included so the
// user sees the full account roster.
export function indexAccountsByConnection(
  accounts: Account[] | undefined,
): Map<string, ConnectionAccountStats> {
  const out = new Map<string, ConnectionAccountStats>();
  if (!accounts) return out;
  for (const a of accounts) {
    if (!a.connection_id) continue;
    let stats = out.get(a.connection_id);
    if (!stats) {
      stats = { count: 0, totalsByCurrency: new Map() };
      out.set(a.connection_id, stats);
    }
    stats.count += 1;
    if (a.is_dependent_linked) continue;
    if (a.balance_current == null || !a.iso_currency_code) continue;
    const prev = stats.totalsByCurrency.get(a.iso_currency_code) ?? 0;
    stats.totalsByCurrency.set(
      a.iso_currency_code,
      prev + a.balance_current,
    );
  }
  return out;
}

// Picks the largest-magnitude currency total to render as the row's headline
// balance. Returns null if no currency has a balance.
export function primaryBalance(
  stats: ConnectionAccountStats | undefined,
): { amount: number; currency: string } | null {
  if (!stats || stats.totalsByCurrency.size === 0) return null;
  let bestCurrency: string | null = null;
  let bestAbs = -1;
  for (const [currency, amount] of stats.totalsByCurrency) {
    if (Math.abs(amount) > bestAbs) {
      bestAbs = Math.abs(amount);
      bestCurrency = currency;
    }
  }
  if (!bestCurrency) return null;
  return {
    amount: stats.totalsByCurrency.get(bestCurrency)!,
    currency: bestCurrency,
  };
}

