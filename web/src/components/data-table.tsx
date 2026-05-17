import { useEffect, useRef, useState } from "react";
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
  type OnChangeFn,
  type RowData,
  type RowSelectionState,
  type SortingState,
} from "@tanstack/react-table";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

// Per-column className, applied to both the header cell and every body cell.
// Lets a column opt into width behaviour (e.g. `w-px` to shrink to content).
declare module "@tanstack/react-table" {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  interface ColumnMeta<TData extends RowData, TValue> {
    className?: string;
  }
}

export interface DataTableProps<TData, TValue> {
  columns: ColumnDef<TData, TValue>[];
  data: TData[];
  isLoading?: boolean;
  isError?: boolean;
  /** Rendered in the table body when data is empty and not loading. */
  emptyState?: React.ReactNode;
  /** Rendered in the table body when isError is true. */
  errorState?: React.ReactNode;
  /** Number of skeleton rows to show while loading. */
  loadingRows?: number;
  /**
   * Optional row-shaped skeleton renderer. When provided it replaces the
   * generic full-width bars with placeholders shaped like the real row, so
   * loading state mirrors the data layout without a layout shift on arrival.
   */
  renderSkeletonRow?: () => React.ReactNode;
  /**
   * Controlled sorting. Owned by the route (typically synced to URL search
   * params), so DataTable stays presentational. Omit for an unsorted table.
   */
  sorting?: SortingState;
  onSortingChange?: OnChangeFn<SortingState>;
  /** Optional row click handler — e.g. navigate to a detail page. */
  onRowClick?: (row: TData) => void;
  /**
   * Controlled row selection. Like sorting, the state lives in the calling
   * route; DataTable just renders it. `getRowId` should return a stable id
   * (not the array index) so a selection survives pagination/refetch.
   */
  enableRowSelection?: boolean;
  rowSelection?: RowSelectionState;
  onRowSelectionChange?: OnChangeFn<RowSelectionState>;
  getRowId?: (row: TData) => string;
  /** Passed through to the table instance — e.g. shift-range-select hooks. */
  meta?: object;
  /**
   * Row id (matching getRowId) to render with a keyboard-focus ring and keep
   * scrolled into view — drives j/k list navigation.
   */
  focusedRowId?: string;
  /**
   * Render the column header as a sticky band that pins flush under the
   * app shell's sticky header (h-14) on long lists. Uses a subtle muted
   * backdrop so it visually separates from the body without a hard line.
   * Pairs well with `transactions.tsx`, `tags-table.tsx`, `api-keys-table.tsx`.
   */
  stickyHeader?: boolean;
  /**
   * When true, render the column headers with the v2 list vocabulary —
   * uppercase, tracked, smaller, muted. The default is the shadcn table's
   * standard sentence-case treatment used by other list pages.
   */
  refinedHeader?: boolean;
}

// Shared table wrapper over @tanstack/react-table + the shadcn Table
// primitive. Pagination and sorting state live in the calling route (URL
// search params) — DataTable only renders. Every v2 list page uses this;
// never hand-roll a <table>.
export function DataTable<TData, TValue>({
  columns,
  data,
  isLoading = false,
  isError = false,
  emptyState,
  errorState,
  loadingRows = 8,
  renderSkeletonRow,
  sorting,
  onSortingChange,
  onRowClick,
  enableRowSelection,
  rowSelection,
  onRowSelectionChange,
  getRowId,
  meta,
  focusedRowId,
  stickyHeader = false,
  refinedHeader = false,
}: DataTableProps<TData, TValue>) {
  const focusedRowRef = useRef<HTMLTableRowElement>(null);
  useEffect(() => {
    focusedRowRef.current?.scrollIntoView({ block: "nearest" });
  }, [focusedRowId]);

  // Detect the sticky header's "stuck" state so the rounded top corners
  // (which match the card while at rest) flatten once the band is
  // floating under the app shell header, and the explicit bottom
  // separator only renders while floating.
  //
  // Observe the `<thead>` directly with a `rootMargin` that excludes the
  // 56px the shell header occupies: while the thead is below that line
  // it's fully inside the (reduced) root → `ratio === 1` → not stuck.
  // The moment it hits the line and `position:sticky` pins it at top:56,
  // its top is above the root and `ratio < 1` → stuck. Sentinels don't
  // work here — a `<div>` directly inside `<table>` gets relocated by
  // the browser and ends up below the thead.
  const headerRef = useRef<HTMLTableSectionElement>(null);
  const [isStuck, setIsStuck] = useState(false);
  useEffect(() => {
    if (!stickyHeader) return;
    const el = headerRef.current;
    if (!el) return;
    const observer = new IntersectionObserver(
      ([entry]) => setIsStuck(entry.intersectionRatio < 1),
      { rootMargin: "-57px 0px 0px 0px", threshold: [1] },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [stickyHeader]);

  const table = useReactTable({
    data,
    columns,
    getCoreRowModel: getCoreRowModel(),
    manualSorting: true,
    state: {
      ...(sorting ? { sorting } : {}),
      ...(rowSelection ? { rowSelection } : {}),
    },
    onSortingChange,
    enableRowSelection,
    onRowSelectionChange,
    getRowId,
    meta,
  });

  const colCount = columns.length;

  return (
    // The shadcn Table primitive normally wraps the <table> in an
    // `overflow-x-auto` container for narrow-viewport horizontal scrolling.
    // That wrapper (plus our own `overflow-hidden` container) creates a
    // scrolling containing block that traps `position: sticky` inside the
    // table — `top-14` is then measured from the inner box, not the
    // viewport, and the header floats inside the table card instead of
    // pinning under the app shell header. When `stickyHeader` is on we
    // suppress both overflow layers so the page itself becomes the sticky
    // ancestor and `top-14` lands flush under the `h-14` app header.
    <div
      className={cn(
        "bg-card rounded-lg border",
        !stickyHeader && "overflow-hidden",
      )}
    >
      <Table containerClassName={stickyHeader ? "overflow-visible" : undefined}>
        <TableHeader
          ref={headerRef}
          className={cn(
            stickyHeader &&
              // top-14 sits the band flush under the app shell's sticky
              // header (`h-14` in `__root.tsx`) so the column labels stay
              // visible without being obscured. z-10 keeps the band above
              // the table body but well below the app header (z-30).
              // Background + border are on the cells (not `<thead>`) so
              // we can round the first/last cell's top corner to match
              // the card's `rounded-lg` at rest, AND so the bottom
              // separator stays visible while stickied (`<tr>` border-b
              // is unreliable in default table-collapse mode).
              "sticky top-14 z-10 [&>tr>th]:bg-muted/40 supports-[backdrop-filter]:[&>tr>th]:bg-muted/30 [&>tr>th]:backdrop-blur-sm [&>tr>th]:transition-[border-radius]",
            // Flat top corners while stuck — the card has scrolled past
            // so rounded ears would just clip into the shell-header bg.
            stickyHeader &&
              !isStuck &&
              "[&>tr>th:first-child]:rounded-tl-lg [&>tr>th:last-child]:rounded-tr-lg",
            // Add an explicit bottom separator only while stuck — at
            // rest the inherited `<tr>` border-b renders fine, and
            // doubling it produced a visibly heavier line.
            stickyHeader &&
              isStuck &&
              "[&>tr>th]:shadow-[inset_0_-1px_0_0_var(--border)]",
          )}
        >
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow
              key={headerGroup.id}
              className={cn(
                // Header rows never react to hover; they're chrome, not data.
                "hover:bg-transparent",
              )}
            >
              {headerGroup.headers.map((header) => (
                <TableHead
                  key={header.id}
                  className={cn(
                    refinedHeader &&
                      "text-muted-foreground h-9 text-[11px] font-medium tracking-[0.06em] uppercase",
                    header.column.columnDef.meta?.className,
                  )}
                >
                  {header.isPlaceholder
                    ? null
                    : flexRender(
                        header.column.columnDef.header,
                        header.getContext(),
                      )}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody>
          {isLoading ? (
            Array.from({ length: loadingRows }).map((_, r) => (
              <TableRow key={`skeleton-${r}`}>
                {renderSkeletonRow
                  ? renderSkeletonRow()
                  : Array.from({ length: colCount }).map((__, c) => (
                      <TableCell key={c}>
                        <Skeleton className="h-4 w-full" />
                      </TableCell>
                    ))}
              </TableRow>
            ))
          ) : isError ? (
            <TableRow>
              <TableCell colSpan={colCount} className="h-32 text-center">
                {errorState ?? (
                  <span className="text-muted-foreground text-sm">
                    Couldn't load this data.
                  </span>
                )}
              </TableCell>
            </TableRow>
          ) : table.getRowModel().rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={colCount} className="h-32 text-center">
                {emptyState ?? (
                  <span className="text-muted-foreground text-sm">
                    Nothing here yet.
                  </span>
                )}
              </TableCell>
            </TableRow>
          ) : (
            table.getRowModel().rows.map((row) => {
              const focused = row.id === focusedRowId;
              return (
                <TableRow
                  key={row.id}
                  ref={focused ? focusedRowRef : undefined}
                  data-state={row.getIsSelected() ? "selected" : undefined}
                  onClick={
                    onRowClick ? () => onRowClick(row.original) : undefined
                  }
                  className={cn(
                    onRowClick && "cursor-pointer",
                    // Keyboard-focus indicator: a 3px primary accent bar on
                    // the left edge plus a faint primary tint, layered via
                    // inset box-shadow so it doesn't shift the row geometry
                    // or compete with the row's selected/hover background.
                    focused &&
                      "bg-primary/[0.04] shadow-[inset_3px_0_0_0_var(--primary)] outline-none",
                    // `scroll-mt-*` so the `scrollIntoView({ block: "nearest" })`
                    // below clears the chrome stacked above the scroll
                    // container: 56px shell header always, +36px when the
                    // table's own header is sticky on top of it. Without
                    // this, scrolling up via `k` lands the focused row
                    // behind the sticky bands.
                    stickyHeader ? "scroll-mt-24" : "scroll-mt-16",
                  )}
                >
                  {row.getVisibleCells().map((cell) => (
                    <TableCell
                      key={cell.id}
                      className={cell.column.columnDef.meta?.className}
                    >
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext(),
                      )}
                    </TableCell>
                  ))}
                </TableRow>
              );
            })
          )}
        </TableBody>
      </Table>
    </div>
  );
}
