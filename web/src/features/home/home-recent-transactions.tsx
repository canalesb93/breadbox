import { Link } from "@tanstack/react-router";
import { ArrowRight, Receipt } from "lucide-react";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/empty-state";
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
  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b py-4">
        <CardTitle className="text-sm font-medium">
          Recent activity
        </CardTitle>
        <CardAction>
          <Button asChild variant="ghost" size="sm" className="-mr-2 h-8 px-2">
            <Link
              to="/transactions"
              className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1 text-xs"
            >
              View all
              <ArrowRight className="size-3.5" />
            </Link>
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="px-0 py-0">
        {isLoading ? (
          <RecentSkeleton />
        ) : !transactions || transactions.length === 0 ? (
          <EmptyState
            icon={Receipt}
            title="No transactions yet"
            description="Connect a bank or import a CSV to start seeing activity here."
          />
        ) : (
          <ul className="divide-y">
            {transactions.map((t) => (
              <li key={t.id}>
                <Link
                  to="/transactions/$txId"
                  params={{ txId: t.short_id }}
                  className="hover:bg-muted/50 focus-visible:bg-muted/50 flex min-w-0 items-center gap-4 px-5 py-3 outline-none transition-colors"
                >
                  <div className="min-w-0 flex-1">
                    <TransactionPrimary transaction={t} />
                  </div>
                  <TransactionAmount transaction={t} />
                </Link>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

function RecentSkeleton() {
  return (
    <ul className="divide-y">
      {[0, 1, 2, 3, 4].map((i) => (
        <li
          key={i}
          className="flex items-center gap-4 px-5 py-3.5"
        >
          <Skeleton className="size-9 rounded-md" />
          <div className="flex-1 space-y-1.5">
            <Skeleton className="h-3.5 w-44" />
            <Skeleton className="h-3 w-28" />
          </div>
          <div className="space-y-1.5 text-right">
            <Skeleton className="ml-auto h-3.5 w-16" />
            <Skeleton className="ml-auto h-3 w-10" />
          </div>
        </li>
      ))}
    </ul>
  );
}
