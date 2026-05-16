import { ArrowDownRight, ArrowUpRight, Wallet } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { formatBalance } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Account } from "@/api/types";
import { isLiability, totalsByCurrency } from "./account-utils";

interface AccountsSummaryProps {
  accounts: Account[];
}

// AccountsSummary is the three-stat strip at the top of /accounts: net
// worth (assets − liabilities) for the primary currency, total assets,
// total liabilities. Multi-currency households are rare in practice; when
// they show up, the strip falls back to the most-populated currency and
// notes the rest in the subline.
export function AccountsSummary({ accounts }: AccountsSummaryProps) {
  const totals = totalsByCurrency(accounts);
  if (totals.length === 0) return null;

  const primary = totals[0];
  const others = totals.slice(1);

  const { assets, liabilities } = accounts.reduce(
    (acc, a) => {
      if (a.is_dependent_linked) return acc;
      if (a.balance_current == null) return acc;
      if (a.iso_currency_code !== primary.currency) return acc;
      if (isLiability(a.type)) acc.liabilities += a.balance_current;
      else acc.assets += a.balance_current;
      return acc;
    },
    { assets: 0, liabilities: 0 },
  );

  // Mobile layout: stack Net worth on its own (full-bleed, dominant) and
  // pair Assets + Liabilities side-by-side underneath in a 2-col grid. The
  // previous single-column stack of three full-width cards consumed the
  // whole viewport before reaching the actual accounts list. From `sm` up
  // we return to the original 3-col strip.
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
      <SummaryStat
        icon={Wallet}
        label="Net worth"
        value={formatBalance(primary.total, primary.currency)}
        sublabel={
          others.length > 0
            ? `+ ${others.length} other currenc${others.length === 1 ? "y" : "ies"}`
            : `${primary.count} accounts`
        }
        tone="primary"
        className="col-span-2 sm:col-span-1"
      />
      <SummaryStat
        icon={ArrowUpRight}
        label="Assets"
        value={formatBalance(assets, primary.currency)}
        tone="success"
      />
      <SummaryStat
        icon={ArrowDownRight}
        label="Liabilities"
        value={formatBalance(liabilities, primary.currency)}
        tone="muted"
      />
    </div>
  );
}

interface SummaryStatProps {
  icon: typeof Wallet;
  label: string;
  value: string;
  sublabel?: string;
  tone: "primary" | "success" | "muted";
  className?: string;
}

function SummaryStat({ icon: Icon, label, value, sublabel, tone, className }: SummaryStatProps) {
  // Override the Card primitive's default `py-6` — these stat cards are
  // dense one-liners, not full-bleed content panels, so they need just
  // enough breathing room to feel like a card without dominating the
  // top of the page.
  return (
    <Card className={cn("py-4", className)}>
      <CardContent className="flex items-center gap-3 px-4 sm:px-6">
        <div
          className={cn(
            "flex size-9 shrink-0 items-center justify-center rounded-lg",
            tone === "primary" && "bg-primary/10 text-primary",
            tone === "success" && "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
            tone === "muted" && "bg-muted text-muted-foreground",
          )}
        >
          <Icon className="size-4" />
        </div>
        <div className="min-w-0 leading-tight">
          <div className="text-muted-foreground text-xs">{label}</div>
          <div className="truncate text-lg font-semibold tabular-nums">
            {value}
          </div>
          {sublabel && (
            <div className="text-muted-foreground mt-0.5 truncate text-[10px]">
              {sublabel}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
