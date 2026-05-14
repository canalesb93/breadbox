import { useEffect, useRef } from "react";
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
  sorting,
  onSortingChange,
  onRowClick,
  enableRowSelection,
  rowSelection,
  onRowSelectionChange,
  getRowId,
  meta,
  focusedRowId,
}: DataTableProps<TData, TValue>) {
  const focusedRowRef = useRef<HTMLTableRowElement>(null);
  useEffect(() => {
    focusedRowRef.current?.scrollIntoView({ block: "nearest" });
  }, [focusedRowId]);

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
    // The shadcn Table primitive already wraps the <table> in its own
    // overflow-x-auto container, so narrow viewports scroll horizontally
    // without a second scroll container here.
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead
                  key={header.id}
                  className={header.column.columnDef.meta?.className}
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
                {Array.from({ length: colCount }).map((__, c) => (
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
                onClick={onRowClick ? () => onRowClick(row.original) : undefined}
                className={cn(
                  onRowClick && "cursor-pointer",
                  focused && "ring-primary ring-2 ring-inset outline-none",
                )}
              >
                {row.getVisibleCells().map((cell) => (
                  <TableCell
                    key={cell.id}
                    className={cell.column.columnDef.meta?.className}
                  >
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
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
