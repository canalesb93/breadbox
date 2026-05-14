import { useEffect, useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { z } from "zod";
import type { ColumnDef } from "@tanstack/react-table";
import { Receipt, Search } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { DataTable } from "@/components/data-table";
import { EmptyState } from "@/components/empty-state";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useTransactions } from "@/api/queries/transactions";
import { flattenPages } from "@/lib/pagination";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import type { Transaction } from "@/api/types";

// Merges with the root route's baseSearchSchema (m/ms modal params).
export const transactionsSearchSchema = z.object({
  q: z.string().optional(),
});

type TransactionsSearch = z.infer<typeof transactionsSearchSchema>;

function formatDate(date: string): string {
  const d = new Date(`${date}T00:00:00`);
  return Number.isNaN(d.getTime())
    ? date
    : d.toLocaleDateString("en-US", { month: "short", day: "numeric" });
}

function formatAmount(amount: number, currency: string | null): string {
  const formatted = new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: currency ?? "USD",
  }).format(Math.abs(amount));
  // Sign convention: positive = money out, negative = money in.
  return amount < 0 ? `+${formatted}` : formatted;
}

const columns: ColumnDef<Transaction>[] = [
  {
    accessorKey: "date",
    header: "Date",
    cell: ({ row }) => (
      <span className="text-muted-foreground tabular-nums">
        {formatDate(row.original.date)}
      </span>
    ),
  },
  {
    id: "merchant",
    header: "Merchant",
    cell: ({ row }) => {
      const t = row.original;
      return (
        <div className="flex items-center gap-2">
          <span className="font-medium">
            {t.provider_merchant_name ?? t.provider_name}
          </span>
          {t.pending && (
            <Badge variant="outline" className="text-muted-foreground">
              Pending
            </Badge>
          )}
        </div>
      );
    },
  },
  {
    id: "category",
    header: "Category",
    cell: ({ row }) => {
      const cat = row.original.category;
      if (!cat?.display_name) {
        return <span className="text-muted-foreground">—</span>;
      }
      return <Badge variant="secondary">{cat.display_name}</Badge>;
    },
  },
  {
    accessorKey: "account_name",
    header: "Account",
    cell: ({ row }) => (
      <span className="text-muted-foreground">
        {row.original.account_name ?? "—"}
      </span>
    ),
  },
  {
    accessorKey: "amount",
    header: () => <div className="text-right">Amount</div>,
    cell: ({ row }) => {
      const t = row.original;
      const isInflow = t.amount < 0;
      return (
        <div
          className={
            isInflow
              ? "text-right font-medium tabular-nums text-emerald-600 dark:text-emerald-500"
              : "text-right font-medium tabular-nums"
          }
        >
          {formatAmount(t.amount, t.iso_currency_code)}
        </div>
      );
    },
  },
];

export function TransactionsPage() {
  const navigate = useNavigate();
  const search = useSearch({ strict: false }) as TransactionsSearch;

  // Local input state → debounced → URL search param. The query is keyed on
  // the URL value, so search is bookmarkable and survives reload.
  const [query, setQuery] = useState(search.q ?? "");
  const debounced = useDebouncedValue(query, 300);

  useEffect(() => {
    const next = debounced.trim() || undefined;
    if (next === (search.q ?? undefined)) return;
    navigate({
      to: ".",
      search: (prev: Record<string, unknown>) => ({ ...prev, q: next }),
    });
  }, [debounced, navigate, search.q]);

  const transactions = useTransactions({ search: search.q });
  const rows = flattenPages<Transaction>(transactions.data?.pages, "transactions");

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
        columns={columns}
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
