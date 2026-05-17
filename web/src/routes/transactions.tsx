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
import { Plus, Receipt } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/data-table";
import { EmptyState } from "@/components/empty-state";
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
import { TransactionsPagination } from "@/features/transactions/transactions-pagination";
import { TransactionRowSkeleton } from "@/features/transactions/transaction-row-skeleton";
import { TagCommandList } from "@/components/tag-command";
import {
  PAGE_LIMIT,
  fetchAllMatchingTransactionIds,
  useTransactionsPage,
  useTransactionCount,
  useUpdateTransactions,
} from "@/api/queries/transactions";
import type { TransactionFilters } from "@/api/queries/transactions";
import { toast } from "sonner";
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
  /** 1-indexed page number for the offset-paginated list view. */
  p: z.coerce.number().int().min(1).optional(),
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
    // Gate on the live URL: if the user already navigated away (e.g. j/k +
    // Enter into a transaction detail before the 300ms debounce fires),
    // skip. Without this, `navigate({ to: "." })` resolves to this
    // component's matched route (`/transactions`) and pushes us back to
    // the list, flashing the URL away from the detail page we just opened.
    if (!window.location.pathname.endsWith("/transactions")) return;
    const q = debounced.trim() || undefined;
    navigate({
      to: ".",
      search: (prev: Record<string, unknown>) => ({ ...prev, q, p: undefined }),
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
        // Any filter change collapses back to page 1 — staying on page 11
        // when the filtered result has only 2 pages would land on an empty
        // view. The pagination control itself patches `p` directly and
        // doesn't go through setFilter.
        search: (prev: Record<string, unknown>) => ({
          ...prev,
          ...patch,
          p: undefined,
        }),
      });
    },
    [navigate],
  );

  const filters = searchToFilters(search);
  const page = search.p ?? 1;
  const transactions = useTransactionsPage(filters, page, PAGE_LIMIT);
  const totalCount = useTransactionCount(filters);
  const rows = useMemo<Transaction[]>(
    () => transactions.data?.transactions ?? [],
    [transactions.data?.transactions],
  );
  const goToPage = useCallback(
    (next: number) => {
      navigate({
        to: ".",
        search: (prev: Record<string, unknown>) => ({
          ...prev,
          p: next > 1 ? next : undefined,
        }),
      });
    },
    [navigate],
  );

  // --- Select mode ---
  const [selectMode, setSelectMode] = useState(false);
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const lastIndexRef = useRef<number | null>(null);
  // Keyboard-navigated row focus (j/k). Index into `rows`; null = no focus.
  const [focusedIndex, setFocusedIndex] = useState<number | null>(null);

  const openTransaction = useCallback(
    (t: Transaction) =>
      navigate({ to: "/transactions/$id", params: { id: t.short_id } }),
    [navigate],
  );

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

  // Row click outside select mode is a two-stage interaction:
  //   1st tap → focus the row (same affordance as j/k)
  //   2nd tap on the already-focused row → enter select mode + select it
  //     (same affordance as the `x` shortcut)
  // The merchant title remains the dedicated open-detail target — its
  // own onClick stops propagation so it never lands here.
  const handleRowClick = useCallback(
    (t: Transaction) => {
      if (selectMode) {
        toggleRowSelection(t);
        return;
      }
      const idx = rows.findIndex((r) => r.id === t.id);
      if (idx === -1) return;
      if (focusedIndex === idx) {
        setSelectMode(true);
        setRowSelection((prev) => ({ ...prev, [t.id]: true }));
        lastIndexRef.current = idx;
        return;
      }
      setFocusedIndex(idx);
    },
    [selectMode, toggleRowSelection, rows, focusedIndex],
  );

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
      onOpenDetail: openTransaction,
    }),
    [rows, openTransaction],
  );

  const focusedRowId =
    focusedIndex != null ? rows[focusedIndex]?.id : undefined;

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
    { label: "Move focus down", group: "Transactions", repeat: true },
  );
  useShortcut(
    ["k"],
    () => setFocusedIndex((i) => (i == null ? 0 : Math.max(i - 1, 0))),
    { label: "Move focus up", group: "Transactions", repeat: true },
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

  // `c` and `t` open centered command dialogs targeting either the bulk
  // selection (when select mode has selected rows) or the j/k-focused row.
  const updateTransactions = useUpdateTransactions();
  const [categorizeOpen, setCategorizeOpen] = useState(false);
  const [tagOpen, setTagOpen] = useState(false);
  const bulkTargets = useMemo(() => {
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
      if (bulkTargets.length === 0) return;
      // Otherwise the same keypress that triggers us also lands inside the
      // dialog's auto-focused command input as a literal "c".
      e.preventDefault();
      setCategorizeOpen(true);
    },
    {
      label: "Categorize focused / selected",
      group: "Transactions",
      enabled: bulkTargets.length > 0,
    },
  );
  useShortcut(
    ["t"],
    (e) => {
      if (bulkTargets.length === 0) return;
      e.preventDefault();
      setTagOpen(true);
    },
    {
      label: "Tag focused / selected",
      group: "Transactions",
      enabled: bulkTargets.length > 0,
    },
  );

  const handleCategorizePick = useCallback(
    (pick: CategoryPick) => {
      if (!bulkTargets.length) return;
      setCategorizeOpen(false);
      const n = bulkTargets.length;
      const plural = n === 1 ? "" : "s";
      const message = pick.reset_category
        ? `Category reset on ${n} transaction${plural}.`
        : `Category applied to ${n} transaction${plural}.`;
      applyBulkTransactionOp(updateTransactions, bulkTargets, pick, message);
    },
    [bulkTargets, updateTransactions],
  );

  const handleTagPick = useCallback(
    (slug: string) => {
      if (!bulkTargets.length) return;
      setTagOpen(false);
      const n = bulkTargets.length;
      const plural = n === 1 ? "" : "s";
      applyBulkTransactionOp(
        updateTransactions,
        bulkTargets,
        { tags_to_add: [{ slug }] },
        `Tag applied to ${n} transaction${plural}.`,
      );
    },
    [bulkTargets, updateTransactions],
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
  const eyebrow = (() => {
    if (transactions.isLoading) return "Loading";
    if (transactions.isError) return "Error";
    if (rows.length === 0) {
      return hasActiveFilters ? "No matches" : "No transactions";
    }
    if (count != null && count > rows.length) {
      return `Showing ${rows.length.toLocaleString()} of ${count.toLocaleString()}`;
    }
    const total = count ?? rows.length;
    return `${total.toLocaleString()} ${total === 1 ? "transaction" : "transactions"}`;
  })();

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        eyebrow={eyebrow}
        title="Transactions"
        description="Search, filter, and review every transaction across your connected accounts."
        actions={
          <Button asChild size="sm">
            <Link
              to="/connections"
              search={{ action: "connect" }}
              className="inline-flex items-center gap-1.5"
            >
              <Plus className="size-4" />
              Connect bank
            </Link>
          </Button>
        }
      />

      <div className="flex flex-col gap-3">
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

      <DataTable
        columns={columns}
        data={rows}
        isLoading={transactions.isLoading}
        isError={transactions.isError}
        loadingRows={8}
        renderSkeletonRow={() => (
          <TransactionRowSkeleton showSelect={selectMode} />
        )}
        getRowId={(t) => t.id}
        enableRowSelection={selectMode}
        rowSelection={selectMode ? rowSelection : undefined}
        onRowSelectionChange={setRowSelection}
        meta={tableMeta}
        focusedRowId={focusedRowId}
        onRowClick={handleRowClick}
        pointerRows={selectMode}
        stickyHeader
        refinedHeader
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
                ? "Try adjusting your filters, or clear them to see every transaction."
                : "Once a connected account finishes syncing, transactions will land here — usually within a minute."
            }
            action={
              hasActiveFilters ? undefined : (
                <Button asChild size="sm">
                  <Link
                    to="/connections"
                    search={{ action: "connect" }}
                    className="inline-flex items-center gap-1.5"
                  >
                    <Plus className="size-4" />
                    Connect a bank
                  </Link>
                </Button>
              )
            }
          />
        }
      />

      {count != null && count > PAGE_LIMIT && (
        <TransactionsPagination
          page={page}
          pageSize={PAGE_LIMIT}
          total={count}
          onPageChange={goToPage}
          isFetching={transactions.isFetching}
        />
      )}

      {selectMode && selectedIds.length > 0 && (
        <SelectionActionBar
          selectedIds={selectedIds}
          totalCount={count}
          onClear={exitSelectMode}
          onSelectAllMatching={async () => {
            try {
              const ids = await fetchAllMatchingTransactionIds(filters);
              setRowSelection(
                Object.fromEntries(ids.map((id) => [id, true])),
              );
              if (count != null && ids.length < count) {
                toast.info(
                  `Selected the first ${ids.length.toLocaleString()} of ${count.toLocaleString()} matches.`,
                );
              }
            } catch {
              toast.error("Couldn't select all matching transactions.");
            }
          }}
        />
      )}

      <CommandDialog
        open={categorizeOpen}
        onOpenChange={setCategorizeOpen}
        title="Categorize"
        description={
          bulkTargets.length === 1
            ? "Apply a category to the focused transaction."
            : `Apply a category to ${bulkTargets.length} selected transactions.`
        }
      >
        <CategoryCommandList onPick={handleCategorizePick} />
      </CommandDialog>

      <CommandDialog
        open={tagOpen}
        onOpenChange={setTagOpen}
        title="Tag"
        description={
          bulkTargets.length === 1
            ? "Add a tag to the focused transaction."
            : `Add a tag to ${bulkTargets.length} selected transactions.`
        }
      >
        <TagCommandList onPick={handleTagPick} />
      </CommandDialog>
    </div>
  );
}
