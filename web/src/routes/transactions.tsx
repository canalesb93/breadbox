import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { z } from "zod";
import type { RowSelectionState } from "@tanstack/react-table";
import { CheckSquare, Receipt, Search, X } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { DataTable } from "@/components/data-table";
import { EmptyState } from "@/components/empty-state";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import {
  buildTransactionColumns,
  type TransactionTableMeta,
} from "@/features/transactions/columns";
import { FilterBar } from "@/features/transactions/filter-bar";
import { SelectionActionBar } from "@/features/transactions/selection-action-bar";
import { useTransactions } from "@/api/queries/transactions";
import type { TransactionFilters } from "@/api/queries/transactions";
import { flattenPages } from "@/lib/pagination";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import type { Transaction } from "@/api/types";

// Merges with the root route's baseSearchSchema (m/ms modal params). Every
// filter is a URL search param so the view is bookmarkable and shareable.
export const transactionsSearchSchema = z.object({
  q: z.string().optional(),
  account: z.string().optional(),
  category: z.string().optional(),
  start: z.string().optional(),
  end: z.string().optional(),
  min: z.coerce.number().optional(),
  max: z.coerce.number().optional(),
  pending: z.enum(["true", "false"]).optional(),
  sort: z.enum(["date", "amount"]).optional(),
  dir: z.enum(["asc", "desc"]).optional(),
});

export type TransactionsSearch = z.infer<typeof transactionsSearchSchema>;

// searchToFilters maps the URL search params onto the query hook's filter
// shape — the one place the param names and the API param names meet.
function searchToFilters(search: TransactionsSearch): TransactionFilters {
  return {
    search: search.q,
    account: search.account,
    category: search.category,
    start: search.start,
    end: search.end,
    minAmount: search.min,
    maxAmount: search.max,
    pending:
      search.pending === "true"
        ? true
        : search.pending === "false"
          ? false
          : undefined,
    sortBy: search.sort,
    sortOrder: search.dir,
  };
}

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

  const setFilter = useCallback(
    (patch: Partial<TransactionsSearch>) => {
      navigate({
        to: ".",
        search: (prev: Record<string, unknown>) => ({ ...prev, ...patch }),
      });
    },
    [navigate],
  );

  const transactions = useTransactions(searchToFilters(search));
  const rows = useMemo(
    () => flattenPages<Transaction>(transactions.data?.pages, "transactions"),
    [transactions.data?.pages],
  );

  // --- Select mode ---
  const [selectMode, setSelectMode] = useState(false);
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const lastIndexRef = useRef<number | null>(null);

  const columns = useMemo(
    () => buildTransactionColumns({ selection: selectMode }),
    [selectMode],
  );

  const exitSelectMode = useCallback(() => {
    setSelectMode(false);
    setRowSelection({});
    lastIndexRef.current = null;
  }, []);

  // getRowId returns the transaction id, so rowSelection keys ARE ids.
  const selectedIds = useMemo(
    () => Object.keys(rowSelection).filter((id) => rowSelection[id]),
    [rowSelection],
  );

  // Shift-click range select: every row between the last-toggled row and the
  // shift-clicked one becomes selected.
  const tableMeta: TransactionTableMeta = useMemo(
    () => ({
      setLastIndex: (index) => {
        lastIndexRef.current = index;
      },
      onRangeSelect: (toIndex) => {
        const from = lastIndexRef.current;
        if (from == null) return;
        const [lo, hi] = from < toIndex ? [from, toIndex] : [toIndex, from];
        setRowSelection((prev) => {
          const next = { ...prev };
          for (let i = lo; i <= hi; i++) {
            const id = rows[i]?.id;
            if (id) next[id] = true;
          }
          return next;
        });
      },
    }),
    [rows],
  );

  const hasActiveFilters =
    !!search.q ||
    !!search.account ||
    !!search.category ||
    !!search.start ||
    !!search.end ||
    search.min != null ||
    search.max != null ||
    !!search.pending;

  return (
    <div>
      <PageHeader
        title="Transactions"
        description="Every transaction synced across your connected accounts."
        actions={
          selectMode ? (
            <Button variant="secondary" size="sm" onClick={exitSelectMode}>
              <X className="size-4" />
              Done
            </Button>
          ) : (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setSelectMode(true)}
            >
              <CheckSquare className="size-4" />
              Select
            </Button>
          )
        }
      />

      <div className="mb-4 space-y-3">
        <div className="relative w-full max-w-xs">
          <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search merchant or description…"
            className="pl-8"
          />
        </div>
        <FilterBar search={search} onChange={setFilter} />
      </div>

      <DataTable
        columns={columns}
        data={rows}
        isLoading={transactions.isLoading}
        isError={transactions.isError}
        getRowId={(t) => t.id}
        enableRowSelection={selectMode}
        rowSelection={selectMode ? rowSelection : undefined}
        onRowSelectionChange={setRowSelection}
        meta={tableMeta}
        onRowClick={
          selectMode
            ? undefined
            : (t) =>
                navigate({
                  to: "/transactions/$id",
                  params: { id: t.short_id },
                })
        }
        emptyState={
          <EmptyState
            icon={Receipt}
            title={
              hasActiveFilters
                ? "No matching transactions"
                : "No transactions yet"
            }
            description={
              hasActiveFilters
                ? "Try adjusting or clearing your filters."
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

      {selectMode && selectedIds.length > 0 && (
        <SelectionActionBar
          selectedIds={selectedIds}
          onClear={exitSelectMode}
        />
      )}
    </div>
  );
}
