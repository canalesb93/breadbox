import { ArrowDownRight, ArrowUpRight, Wallet } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import type { Account } from "@/api/types";
import { formatCurrency, isLiability, totalsByCurrency } from "./account-utils";

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

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
      <SummaryStat
        icon={Wallet}
        label="Net worth"
        value={formatCurrency(primary.total, primary.currency)}
        sublabel={
          others.length > 0
            ? `+ ${others.length} other currenc${others.length === 1 ? "y" : "ies"}`
            : `${primary.count} accounts`
        }
        tone="primary"
      />
      <SummaryStat
        icon={ArrowUpRight}
        label="Assets"
        value={formatCurrency(assets, primary.currency)}
        tone="success"
      />
      <SummaryStat
        icon={ArrowDownRight}
        label="Liabilities"
        value={formatCurrency(liabilities, primary.currency)}
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
}

function SummaryStat({ icon: Icon, label, value, sublabel, tone }: SummaryStatProps) {
  // Override the Card primitive's default `py-6` — these stat cards are
  // dense one-liners, not full-bleed content panels, so they need just
  // enough breathing room to feel like a card without dominating the
  // top of the page.
  return (
    <Card className="py-4">
      <CardContent className="flex items-center gap-3">
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
