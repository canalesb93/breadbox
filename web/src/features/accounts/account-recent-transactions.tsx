import { Link } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { EmptyState } from "@/components/empty-state";
import { TransactionPrimary } from "@/components/transaction-primary";
import { TransactionAmount } from "@/components/transaction-amount";
import type { Transaction } from "@/api/types";

interface AccountRecentTransactionsProps {
  accountShortId: string;
  transactions: Transaction[];
}

// AccountRecentTransactions surfaces the last N transactions inline on the
// account detail page. Each row links to the transaction detail; the footer
// jumps to the Transactions table pre-filtered to this account.
export function AccountRecentTransactions({
  accountShortId,
  transactions,
}: AccountRecentTransactionsProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Recent transactions</CardTitle>
        {transactions.length > 0 && (
          <CardAction>
            <span className="text-muted-foreground text-xs">
              Last {transactions.length}
            </span>
          </CardAction>
        )}
      </CardHeader>
      <CardContent className="p-0">
        {transactions.length === 0 ? (
          <EmptyState
            title="No transactions yet"
            description="They appear after the first sync."
            className="py-8"
          />
        ) : (
          <ul className="divide-y">
            {transactions.map((t) => (
              <li key={t.id}>
                <Link
                  to="/transactions/$id"
                  params={{ id: t.short_id }}
                  className="hover:bg-accent/40 flex items-center gap-3 px-6 py-3 transition-colors"
                >
                  <TransactionPrimary transaction={t} className="flex-1" />
                  <TransactionAmount transaction={t} />
                </Link>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
      {transactions.length > 0 && (
        <div className="border-border/40 border-t px-6 py-3 text-right">
          <Button variant="ghost" size="sm" asChild>
            <Link
              to="/transactions"
              search={{ account: accountShortId }}
            >
              See all transactions for this account
              <ArrowRight className="size-3.5" />
            </Link>
          </Button>
        </div>
      )}
    </Card>
  );
}
