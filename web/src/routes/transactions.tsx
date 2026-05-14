import { useEffect, useMemo, useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { z } from "zod";
import { Receipt, Search } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { DataTable } from "@/components/data-table";
import { EmptyState } from "@/components/empty-state";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { transactionColumns } from "@/features/transactions/columns";
import { useTransactions } from "@/api/queries/transactions";
import { flattenPages } from "@/lib/pagination";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import type { Transaction } from "@/api/types";

// Merges with the root route's baseSearchSchema (m/ms modal params).
export const transactionsSearchSchema = z.object({
  q: z.string().optional(),
});

type TransactionsSearch = z.infer<typeof transactionsSearchSchema>;

export function TransactionsPage() {
  const navigate = useNavigate();
  const search = useSearch({ strict: false }) as TransactionsSearch;

  // `query` is local input state for responsive typing. It's debounced, then
  // pushed to the URL (the bookmarkable source of truth). The forward effect
  // is keyed only on `debounced` — never on `search.q` — so an external URL
  // change (back/forward, command palette) doesn't re-trigger a push and
  // fight the reverse sync below.
  const [query, setQuery] = useState(search.q ?? "");
  const debounced = useDebouncedValue(query, 300);

  useEffect(() => {
    const q = debounced.trim() || undefined;
    navigate({
      to: ".",
      search: (prev: Record<string, unknown>) => ({ ...prev, q }),
    });
  }, [debounced, navigate]);

  // Adopt the URL value when it changes from outside this component.
  // setQuery to an equal value is a no-op, so our own pushes don't loop.
  useEffect(() => {
    setQuery(search.q ?? "");
  }, [search.q]);

  const transactions = useTransactions({ search: search.q });
  const rows = useMemo(
    () => flattenPages<Transaction>(transactions.data?.pages, "transactions"),
    [transactions.data?.pages],
  );

  return (
    <div>
      <PageHeader
        title="Transactions"
        description="Every transaction synced across your connected accounts."
      />

      <div className="mb-4 flex items-center gap-2">
        <div className="relative w-full max-w-xs">
          <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search merchant or description…"
            className="pl-8"
          />
        </div>
      </div>

      <DataTable
        columns={transactionColumns}
        data={rows}
        isLoading={transactions.isLoading}
        isError={transactions.isError}
        emptyState={
          <EmptyState
            icon={Receipt}
            title={search.q ? "No matching transactions" : "No transactions yet"}
            description={
              search.q
                ? "Try a different search term."
                : "Transactions appear here once an account finishes syncing."
            }
          />
        }
      />

      {transactions.hasNextPage && (
        <div className="mt-4 flex justify-center">
          <Button
            variant="outline"
            onClick={() => transactions.fetchNextPage()}
            disabled={transactions.isFetchingNextPage}
          >
            {transactions.isFetchingNextPage ? "Loading…" : "Load more"}
          </Button>
        </div>
      )}
    </div>
  );
}
