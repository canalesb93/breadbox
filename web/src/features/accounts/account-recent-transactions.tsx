import { Link } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/empty-state";
import { ListCard } from "@/components/list-card";
import { TransactionPrimary } from "@/components/transaction-primary";
import { TransactionAmount } from "@/components/transaction-amount";
import type { Transaction } from "@/api/types";

interface AccountRecentTransactionsProps {
  accountShortId: string;
  transactions: Transaction[];
}

// AccountRecentTransactions surfaces the last N transactions inline on the
// account detail page. Each row links to the transaction detail; the footer
// jumps to the Transactions table pre-filtered to this account. Uses the
// shared ListCard primitive so the bordered-header + divide-y rail vocabulary
// stays unified across every list surface in the v2 SPA.
export function AccountRecentTransactions({
  accountShortId,
  transactions,
}: AccountRecentTransactionsProps) {
  const hasRows = transactions.length > 0;
  return (
    <ListCard
      title="Recent transactions"
      action={
        hasRows ? (
          <span className="text-muted-foreground text-xs">
            Last {transactions.length}
          </span>
        ) : undefined
      }
      rows={transactions}
      getRowKey={(t) => t.id}
      renderRow={(t) => (
        <Link
          to="/transactions/$id"
          params={{ id: t.short_id }}
          className="hover:bg-accent/40 flex items-center gap-3 px-5 py-2.5 transition-colors"
        >
          <TransactionPrimary transaction={t} className="flex-1" />
          <TransactionAmount transaction={t} />
        </Link>
      )}
      empty={
        <EmptyState
          title="No transactions yet"
          description="Transactions for this account will land here after the first successful sync."
          className="py-10"
        />
      }
      footer={
        hasRows ? (
          <Button variant="ghost" size="sm" asChild>
            <Link to="/transactions" search={{ account: accountShortId }}>
              See all transactions for this account
              <ArrowRight className="size-3.5" />
            </Link>
          </Button>
        ) : undefined
      }
    />
  );
}
