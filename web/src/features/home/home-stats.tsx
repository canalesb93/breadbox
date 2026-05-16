import { ArrowDownRight, ArrowUpRight, Wallet } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { ColorRailCard } from "@/components/color-rail-card";
import { formatCompactAmount } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Account, Connection } from "@/api/types";

interface HomeStatsProps {
  accounts: Account[] | undefined;
  connections: Connection[] | undefined;
  isLoading: boolean;
}

// Aggregates the /api/v1/accounts payload into the household-level balance
// summary that anchors the home page. Currency is kept per-bucket so totals
// are never summed across currencies — we show the dominant bucket and a
// "+N currencies" hint when more than one exists. Same convention used by
// the v1 dashboard so the numbers match across surfaces.
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

// HomeStats is the hero balance summary on the home page. A single
// ColorRailCard reads as the page's "first read" — the rail encodes
// solvency (success when net is positive, destructive when underwater)
// and the value uses a much larger tabular-num display so the headline
// number is unambiguous. Two secondary metrics (Cash / Credit & loans)
// sit inside the same card as labelled cells, separated by the card
// border so they read as supporting context rather than four equal
// scoreboard tiles.
//
// Connection health moved into the side connections panel header — the
// number was duplicated here for no reason and the side panel already
// has the right context for it.
export function HomeStats({ accounts, connections, isLoading }: HomeStatsProps) {
  if (isLoading || !accounts || !connections) {
    return <HomeStatsSkeleton />;
  }

  const cash = bucket();
  const credit = bucket();
  // Net = depository minus credit/loan (loans counted as debt, positive
  // balance_current). Same logic as the v1 dashboard summary.
  const netByCurrency = new Map<string, number>();
  let activeAccounts = 0;
  for (const a of accounts) {
    if (a.is_dependent_linked) continue;
    activeAccounts += 1;
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
  const net: CurrencyTotals = { totals: netByCurrency, count: activeAccounts };

  const cashP = primary(cash);
  const creditP = primary(credit);
  const netP = primary(net);
  const positive = netP.amount >= 0;
  const currencyCount = net.totals.size;

  return (
    <ColorRailCard
      accent={positive ? "var(--success)" : "var(--destructive)"}
      cardClassName="shadow-sm"
    >
      <div className="grid gap-0 sm:grid-cols-[1.4fr_1fr_1fr]">
        <HeroCell
          eyebrow={
            <>
              <Wallet className="size-3.5" />
              Net cash
            </>
          }
          value={formatCompactAmount(netP.amount, netP.currency)}
          valueTone={positive ? "positive" : "negative"}
          hint={
            <>
              {activeAccounts} {activeAccounts === 1 ? "account" : "accounts"}
              {currencyCount > 1 && (
                <span className="text-muted-foreground/70">
                  {" "}
                  · {currencyCount} currencies
                </span>
              )}
            </>
          }
          className="px-6 py-6 sm:py-7 sm:pl-7"
        />
        <SecondaryCell
          eyebrow={
            <>
              <ArrowUpRight className="size-3.5" />
              Cash
            </>
          }
          value={formatCompactAmount(cashP.amount, cashP.currency)}
          hint={`${cash.count} ${cash.count === 1 ? "depository" : "depository accounts"}`}
          className="px-6 py-5 sm:border-l sm:py-7"
        />
        <SecondaryCell
          eyebrow={
            <>
              <ArrowDownRight className="size-3.5" />
              Credit &amp; loans
            </>
          }
          value={formatCompactAmount(creditP.amount, creditP.currency)}
          valueTone={creditP.amount > 0 ? "warning" : "neutral"}
          hint={`${credit.count} ${credit.count === 1 ? "balance" : "balances"}`}
          className="px-6 py-5 sm:border-l sm:py-7 sm:pr-7"
        />
      </div>
    </ColorRailCard>
  );
}

interface CellProps {
  eyebrow: React.ReactNode;
  value: string;
  hint?: React.ReactNode;
  valueTone?: "positive" | "negative" | "warning" | "neutral";
  className?: string;
}

const TONE_CLASS: Record<NonNullable<CellProps["valueTone"]>, string> = {
  positive: "text-success",
  negative: "text-destructive",
  warning: "text-amber-600 dark:text-amber-400",
  neutral: "",
};

// HeroCell is the dominant value. Eyebrow + 3xl value + hint.
function HeroCell({ eyebrow, value, hint, valueTone = "neutral", className }: CellProps) {
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <div className="text-muted-foreground inline-flex items-center gap-1.5 text-[11px] font-medium tracking-[0.08em] uppercase">
        {eyebrow}
      </div>
      <div
        className={cn(
          "text-3xl font-semibold tracking-tight tabular-nums sm:text-[2rem] sm:leading-tight",
          TONE_CLASS[valueTone],
        )}
      >
        {value}
      </div>
      {hint && (
        <div className="text-muted-foreground text-xs">{hint}</div>
      )}
    </div>
  );
}

// SecondaryCell shares the cell shape but smaller value, used for the
// supporting metrics inside the hero.
function SecondaryCell({
  eyebrow,
  value,
  hint,
  valueTone = "neutral",
  className,
}: CellProps) {
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <div className="text-muted-foreground inline-flex items-center gap-1.5 text-[11px] font-medium tracking-[0.08em] uppercase">
        {eyebrow}
      </div>
      <div
        className={cn(
          "text-xl font-semibold tracking-tight tabular-nums",
          TONE_CLASS[valueTone],
        )}
      >
        {value}
      </div>
      {hint && (
        <div className="text-muted-foreground text-xs">{hint}</div>
      )}
    </div>
  );
}

function HomeStatsSkeleton() {
  return (
    <ColorRailCard accent="var(--muted)" cardClassName="shadow-sm">
      <div className="grid gap-0 sm:grid-cols-[1.4fr_1fr_1fr]">
        <div className="flex flex-col gap-2 px-6 py-6 sm:py-7 sm:pl-7">
          <Skeleton className="h-3 w-20" />
          <Skeleton className="h-8 w-40 sm:h-9" />
          <Skeleton className="h-3 w-24" />
        </div>
        <div className="flex flex-col gap-2 px-6 py-5 sm:border-l sm:py-7">
          <Skeleton className="h-3 w-16" />
          <Skeleton className="h-6 w-32" />
          <Skeleton className="h-3 w-20" />
        </div>
        <div className="flex flex-col gap-2 px-6 py-5 sm:border-l sm:py-7 sm:pr-7">
          <Skeleton className="h-3 w-24" />
          <Skeleton className="h-6 w-28" />
          <Skeleton className="h-3 w-16" />
        </div>
      </div>
    </ColorRailCard>
  );
}
