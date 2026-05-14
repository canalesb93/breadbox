import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
  type OnChangeFn,
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
}: DataTableProps<TData, TValue>) {
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
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead key={header.id}>
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
            table.getRowModel().rows.map((row) => (
              <TableRow
                key={row.id}
                data-state={row.getIsSelected() ? "selected" : undefined}
                onClick={onRowClick ? () => onRowClick(row.original) : undefined}
                className={cn(onRowClick && "cursor-pointer")}
              >
                {row.getVisibleCells().map((cell) => (
                  <TableCell key={cell.id}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  );
}
