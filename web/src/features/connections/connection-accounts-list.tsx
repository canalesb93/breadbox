import { Link } from "@tanstack/react-router";
import { Banknote, CreditCard, Landmark, PiggyBank, Wallet } from "lucide-react";
import type { Account } from "@/api/types";
import { EmptyState } from "@/components/empty-state";
import { formatBalance } from "@/lib/format";

const TYPE_ICON: Record<string, typeof Banknote> = {
  depository: PiggyBank,
  credit: CreditCard,
  loan: Landmark,
  investment: Wallet,
};

interface ConnectionAccountsListProps {
  accounts: Account[];
}

// Compact list of accounts attached to a connection — each card links to
// the per-account detail page where rename / exclude / linking live.
export function ConnectionAccountsList({ accounts }: ConnectionAccountsListProps) {
  if (accounts.length === 0) {
    return (
      <EmptyState
        variant="inline"
        icon={Wallet}
        title="No accounts yet"
        description="Accounts will land here after the first successful sync — usually within a minute of connecting."
      />
    );
  }
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {accounts.map((a) => (
        <AccountCard key={a.id} account={a} />
      ))}
    </div>
  );
}

function AccountCard({ account: a }: { account: Account }) {
  const Icon = TYPE_ICON[a.type] ?? Banknote;
  return (
    <Link
      to="/accounts/$id"
      params={{ id: a.short_id }}
      className="bg-card hover:bg-accent/40 flex items-center gap-3 rounded-lg border p-3 transition-colors"
    >
      <div className="bg-muted flex size-9 shrink-0 items-center justify-center rounded-lg">
        <Icon className="text-muted-foreground size-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{a.name}</div>
        <div className="text-muted-foreground truncate text-xs">
          <span className="capitalize">{a.subtype ?? a.type}</span>
          {a.mask ? ` · ····${a.mask}` : ""}
        </div>
      </div>
      <div className="text-right text-sm font-semibold tabular-nums whitespace-nowrap">
        {a.balance_current != null && a.iso_currency_code
          ? formatBalance(a.balance_current, a.iso_currency_code)
          : "—"}
      </div>
    </Link>
  );
}
