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
import { Loader2, Receipt } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { DataTable } from "@/components/data-table";
import { EmptyState } from "@/components/empty-state";
import { Button } from "@/components/ui/button";
import { CommandDialog } from "@/components/ui/command";
import {
  CategoryCommandList,
  type CategoryPick,
} from "@/components/category-command";
import {
  buildTransactionColumns,
  type TransactionTableMeta,
} from "@/features/transactions/columns";
import { TransactionsToolbar } from "@/features/transactions/transactions-toolbar";
import { SelectionActionBar } from "@/features/transactions/selection-action-bar";
import { applyBulkTransactionOp } from "@/features/transactions/bulk-update";
import {
  useTransactions,
  useTransactionCount,
  useUpdateTransactions,
} from "@/api/queries/transactions";
import type { TransactionFilters } from "@/api/queries/transactions";
import { flattenPages } from "@/lib/pagination";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { useShortcut } from "@/lib/shortcuts";
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
  const searchInputRef = useRef<HTMLInputElement>(null);

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

  const filters = searchToFilters(search);
  const transactions = useTransactions(filters);
  const totalCount = useTransactionCount(filters);
  const rows = useMemo(
    () => flattenPages<Transaction>(transactions.data?.pages, "transactions"),
    [transactions.data?.pages],
  );

  // --- Select mode ---
  const [selectMode, setSelectMode] = useState(false);
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const lastIndexRef = useRef<number | null>(null);
  // Keyboard-navigated row focus (j/k). Index into `rows`; null = no focus.
  const [focusedIndex, setFocusedIndex] = useState<number | null>(null);

  const columns = useMemo(
    () => buildTransactionColumns({ selection: selectMode }),
    [selectMode],
  );

  const exitSelectMode = useCallback(() => {
    setSelectMode(false);
    setRowSelection({});
    lastIndexRef.current = null;
    setFocusedIndex(null);
  }, []);

  // In select mode a click anywhere on the row toggles its selection — the
  // checkbox cell stops propagation so it doesn't double-toggle.
  const toggleRowSelection = useCallback((t: Transaction) => {
    setRowSelection((prev) => ({ ...prev, [t.id]: !prev[t.id] }));
  }, []);

  const toggleSelectMode = useCallback(() => {
    if (selectMode) exitSelectMode();
    else setSelectMode(true);
  }, [selectMode, exitSelectMode]);

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

  const focusedRowId =
    focusedIndex != null ? rows[focusedIndex]?.id : undefined;

  const openTransaction = useCallback(
    (t: Transaction) =>
      navigate({ to: "/transactions/$id", params: { id: t.short_id } }),
    [navigate],
  );

  // --- Keyboard shortcuts (registered while this page is mounted) ---
  useShortcut(
    ["/"],
    (e) => {
      e.preventDefault();
      searchInputRef.current?.focus();
    },
    { label: "Focus search", group: "Transactions" },
  );
  useShortcut(
    ["j"],
    () =>
      setFocusedIndex((i) =>
        i == null ? 0 : Math.min(i + 1, rows.length - 1),
      ),
    { label: "Move focus down", group: "Transactions" },
  );
  useShortcut(
    ["k"],
    () => setFocusedIndex((i) => (i == null ? 0 : Math.max(i - 1, 0))),
    { label: "Move focus up", group: "Transactions" },
  );
  useShortcut(
    ["enter"],
    () => {
      if (focusedIndex == null) return;
      const t = rows[focusedIndex];
      if (t) openTransaction(t);
    },
    { label: "Open focused transaction", group: "Transactions" },
  );
  useShortcut(
    ["x"],
    () => {
      // No focused row → enter select mode and land the focus ring on the
      // first row, so the next `x` has something to select.
      if (focusedIndex == null) {
        setSelectMode(true);
        setFocusedIndex(0);
        return;
      }
      const id = rows[focusedIndex]?.id;
      if (!id) return;
      setSelectMode(true);
      setRowSelection((prev) => ({ ...prev, [id]: !prev[id] }));
      lastIndexRef.current = focusedIndex;
    },
    { label: "Select focused transaction", group: "Transactions" },
  );
  useShortcut(
    ["escape"],
    () => {
      if (selectedIds.length > 0) setRowSelection({});
      else if (selectMode) exitSelectMode();
      else setFocusedIndex(null);
    },
    { label: "Clear selection / focus", group: "Transactions" },
  );

  // --- Categorize shortcut ---
  // `c` opens a centered command dialog targeting either the bulk selection
  // (when select mode has selected rows) or the j/k-focused row.
  const updateTransactions = useUpdateTransactions();
  const [categorizeOpen, setCategorizeOpen] = useState(false);
  const categorizeTargets = useMemo(() => {
    if (selectedIds.length > 0) return selectedIds;
    if (focusedIndex != null) {
      const id = rows[focusedIndex]?.id;
      return id ? [id] : [];
    }
    return [];
  }, [selectedIds, focusedIndex, rows]);

  useShortcut(
    ["c"],
    (e) => {
      if (categorizeTargets.length === 0) return;
      // Otherwise the same keypress that triggers us also lands inside the
      // dialog's auto-focused command input as a literal "c".
      e.preventDefault();
      setCategorizeOpen(true);
    },
    {
      label: "Categorize focused / selected",
      group: "Transactions",
      enabled: categorizeTargets.length > 0,
    },
  );

  const handleCategorizePick = useCallback(
    (pick: CategoryPick) => {
      if (!categorizeTargets.length) return;
      setCategorizeOpen(false);
      const n = categorizeTargets.length;
      const plural = n === 1 ? "" : "s";
      const message = pick.reset_category
        ? `Category reset on ${n} transaction${plural}.`
        : `Category applied to ${n} transaction${plural}.`;
      applyBulkTransactionOp(updateTransactions, categorizeTargets, pick, message);
    },
    [categorizeTargets, updateTransactions],
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

  const count = totalCount.data?.count;
  const showCountLine = !transactions.isLoading && !transactions.isError;

  return (
    <div>
      <PageHeader title="Transactions" />

      <div className="mb-4">
        <TransactionsToolbar
          search={search}
          onChange={setFilter}
          query={query}
          onQueryChange={setQuery}
          searchRef={searchInputRef}
          selectMode={selectMode}
          onToggleSelect={toggleSelectMode}
        />
      </div>

      {showCountLine && rows.length > 0 && (
        <div className="text-muted-foreground mb-2 flex items-center gap-2 text-sm">
          <span>
            {count != null && count > rows.length
              ? `Showing ${rows.length} of ${count.toLocaleString()} transactions`
              : `${(count ?? rows.length).toLocaleString()} ${
                  (count ?? rows.length) === 1 ? "transaction" : "transactions"
                }`}
          </span>
          {focusedIndex == null && (
            <span className="hidden text-xs sm:inline">
              · Press J / K to navigate
            </span>
          )}
        </div>
      )}

      <DataTable
        columns={columns}
        data={rows}
        isLoading={transactions.isLoading}
        isError={transactions.isError}
        loadingRows={6}
        getRowId={(t) => t.id}
        enableRowSelection={selectMode}
        rowSelection={selectMode ? rowSelection : undefined}
        onRowSelectionChange={setRowSelection}
        meta={tableMeta}
        focusedRowId={focusedRowId}
        onRowClick={selectMode ? toggleRowSelection : openTransaction}
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

      {rows.length > 0 && (
        <div className="text-muted-foreground mt-4 flex justify-center text-sm">
          {transactions.hasNextPage ? (
            <Button
              variant="outline"
              onClick={() => transactions.fetchNextPage()}
              disabled={transactions.isFetchingNextPage}
            >
              {transactions.isFetchingNextPage && (
                <Loader2 className="size-4 animate-spin" />
              )}
              {transactions.isFetchingNextPage ? "Loading…" : "Load more"}
            </Button>
          ) : (
            <span>
              {count != null
                ? `All ${count.toLocaleString()} transactions loaded`
                : "All transactions loaded"}
            </span>
          )}
        </div>
      )}

      {selectMode && selectedIds.length > 0 && (
        <SelectionActionBar
          selectedIds={selectedIds}
          totalCount={count}
          onClear={exitSelectMode}
        />
      )}

      <CommandDialog
        open={categorizeOpen}
        onOpenChange={setCategorizeOpen}
        title="Categorize"
        description={
          categorizeTargets.length === 1
            ? "Apply a category to the focused transaction."
            : `Apply a category to ${categorizeTargets.length} selected transactions.`
        }
      >
        <CategoryCommandList onPick={handleCategorizePick} />
      </CommandDialog>
    </div>
  );
}
