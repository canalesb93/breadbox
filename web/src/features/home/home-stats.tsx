import {
  Banknote,
  CreditCard,
  Plug,
  TrendingUp,
  type LucideIcon,
} from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import type { Account, Connection } from "@/api/types";

interface HomeStatsProps {
  accounts: Account[] | undefined;
  connections: Connection[] | undefined;
  isLoading: boolean;
}

// Aggregates the same /api/v1/accounts payload that the Accounts page reads,
// bucketed into the four card metrics. Currency is kept per-bucket so the
// header rendering never sums across currencies — we just show the largest
// bucket and a "+N more" hint if other currencies exist (rare on a household
// dataset; keeps the card honest without inventing a single number).
interface CurrencyTotals {
  totals: Map<string, number>;
  count: number;
}

function bucket(): CurrencyTotals {
  return { totals: new Map(), count: 0 };
}

function add(b: CurrencyTotals, amount: number | null, currency: string | null) {
  if (amount == null) return;
  const c = currency ?? "USD";
  b.totals.set(c, (b.totals.get(c) ?? 0) + amount);
  b.count += 1;
}

function primary(b: CurrencyTotals): { amount: number; currency: string } {
  if (b.totals.size === 0) return { amount: 0, currency: "USD" };
  let bestC = "USD";
  let bestAbs = -Infinity;
  for (const [c, a] of b.totals) {
    if (Math.abs(a) > bestAbs) {
      bestAbs = Math.abs(a);
      bestC = c;
    }
  }
  return { amount: b.totals.get(bestC) ?? 0, currency: bestC };
}

function formatCompact(amount: number, currency: string): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency,
    maximumFractionDigits: 0,
  }).format(amount);
}

export function HomeStats({ accounts, connections, isLoading }: HomeStatsProps) {
  if (isLoading || !accounts || !connections) {
    return (
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {[0, 1, 2, 3].map((i) => (
          <StatCardSkeleton key={i} />
        ))}
      </div>
    );
  }

  const cash = bucket();
  const credit = bucket();
  // Net = depository minus credit/loan (loans counted as debt, positive
  // balance_current). Same logic as the v1 dashboard summary.
  const netByCurrency = new Map<string, number>();
  for (const a of accounts) {
    if (a.is_dependent_linked) continue;
    const c = a.iso_currency_code ?? "USD";
    const bal = a.balance_current ?? 0;
    if (a.type === "depository") {
      add(cash, bal, c);
      netByCurrency.set(c, (netByCurrency.get(c) ?? 0) + bal);
    } else if (a.type === "credit" || a.type === "loan") {
      add(credit, bal, c);
      netByCurrency.set(c, (netByCurrency.get(c) ?? 0) - bal);
    }
  }
  const net: CurrencyTotals = { totals: netByCurrency, count: accounts.length };

  const healthy = connections.filter((c) => c.status === "active").length;
  const attention = connections.filter(
    (c) => c.status === "pending_reauth" || c.status === "error",
  ).length;

  const cashP = primary(cash);
  const creditP = primary(credit);
  const netP = primary(net);

  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <StatCard
        icon={TrendingUp}
        label="Net cash"
        value={formatCompact(netP.amount, netP.currency)}
        accent={netP.amount >= 0 ? "positive" : "negative"}
        hint={`${accounts.length} ${accounts.length === 1 ? "account" : "accounts"}${net.totals.size > 1 ? ` · ${net.totals.size} currencies` : ""}`}
      />
      <StatCard
        icon={Banknote}
        label="Cash"
        value={formatCompact(cashP.amount, cashP.currency)}
        hint={`${cash.count} depository`}
      />
      <StatCard
        icon={CreditCard}
        label="Credit & loans"
        value={formatCompact(creditP.amount, creditP.currency)}
        accent={creditP.amount > 0 ? "negative" : "neutral"}
        hint={`${credit.count} balance${credit.count === 1 ? "" : "s"}`}
      />
      <StatCard
        icon={Plug}
        label="Connections"
        value={String(connections.length)}
        hint={
          attention > 0
            ? `${healthy} healthy · ${attention} need action`
            : `${healthy} healthy`
        }
        accent={attention > 0 ? "warning" : "neutral"}
      />
    </div>
  );
}

interface StatCardProps {
  icon: LucideIcon;
  label: string;
  value: string;
  hint?: string;
  accent?: "positive" | "negative" | "warning" | "neutral";
}

// One stat tile: tiny uppercase label, big tabular-num value, a footnote line.
// Border-only card — flatter than the default shadcn Card so a row of four
// reads as a scoreboard, not a deck of feature cards.
function StatCard({ icon: Icon, label, value, hint, accent = "neutral" }: StatCardProps) {
  const valueColor =
    accent === "positive"
      ? "text-success"
      : accent === "negative"
        ? "text-destructive"
        : accent === "warning"
          ? "text-amber-600 dark:text-amber-400"
          : undefined;
  return (
    <div className="bg-card rounded-xl border p-5 shadow-sm">
      <div className="text-muted-foreground flex items-center gap-2 text-[11px] font-medium tracking-[0.08em] uppercase">
        <Icon className="size-3.5" />
        {label}
      </div>
      <div
        className={cn(
          "mt-3 text-2xl font-semibold tracking-tight tabular-nums",
          valueColor,
        )}
      >
        {value}
      </div>
      {hint && (
        <div className="text-muted-foreground mt-1 text-xs">{hint}</div>
      )}
    </div>
  );
}

function StatCardSkeleton() {
  return (
    <div className="bg-card rounded-xl border p-5 shadow-sm">
      <Skeleton className="h-3 w-20" />
      <Skeleton className="mt-3 h-7 w-32" />
      <Skeleton className="mt-2 h-3 w-24" />
    </div>
  );
}
