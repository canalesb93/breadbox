import { Link } from "@tanstack/react-router";
import { ArrowRight, Receipt } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/empty-state";
import { ListCard } from "@/components/list-card";
import { ListRowSkeleton } from "@/components/list-row-skeleton";
import { TransactionPrimary } from "@/components/transaction-primary";
import { TransactionAmount } from "@/components/transaction-amount";
import type { Transaction } from "@/api/types";

interface HomeRecentTransactionsProps {
  transactions: Transaction[] | undefined;
  isLoading: boolean;
}

// Recent activity feed for the home page. Mirrors the row style of the
// Transactions list (TransactionPrimary + TransactionAmount) so the user's
// scanning vocabulary doesn't change between the two surfaces. Rows link to
// the transaction detail, and the card header links to the full list.
export function HomeRecentTransactions({
  transactions,
  isLoading,
}: HomeRecentTransactionsProps) {
  const viewAll = (
    <Button asChild variant="ghost" size="sm" className="-mr-2 h-8 px-2">
      <Link
        to="/transactions"
        className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1 text-xs"
      >
        View all
        <ArrowRight className="size-3.5" />
      </Link>
    </Button>
  );

  if (isLoading) {
    return (
      <ListCard
        title="Recent activity"
        action={viewAll}
        rows={[0, 1, 2, 3, 4]}
        getRowKey={(i) => i}
        renderRow={() => (
          <ListRowSkeleton
            density="regular"
            leading="sm-square"
            lines={2}
            trailing="value-stack"
            titleClassName="w-44"
            subtitleClassName="w-28"
            trailingTopClassName="w-16"
            trailingBottomClassName="w-10"
          />
        )}
      />
    );
  }

  return (
    <ListCard
      title="Recent activity"
      action={viewAll}
      rows={transactions ?? []}
      getRowKey={(t) => t.id}
      renderRow={(t) => (
        <Link
          to="/transactions/$txId"
          params={{ txId: t.short_id }}
          className="hover:bg-muted/40 focus-visible:bg-muted/40 flex min-w-0 items-center gap-4 px-5 py-3 outline-none transition-colors"
        >
          <div className="min-w-0 flex-1">
            <TransactionPrimary transaction={t} />
          </div>
          <TransactionAmount transaction={t} />
        </Link>
      )}
      empty={
        <EmptyState
          icon={Receipt}
          title="No transactions yet"
          description="Connect a bank or import a CSV — fresh activity will start landing here within a minute."
        />
      }
    />
  );
}
