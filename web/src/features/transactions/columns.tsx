import type { ColumnDef } from "@tanstack/react-table";
import { Checkbox } from "@/components/ui/checkbox";
import { CategoryPicker } from "@/components/category-picker";
import { TransactionPrimary } from "@/components/transaction-primary";
import { TransactionAmount } from "@/components/transaction-amount";
import type { Transaction } from "@/api/types";

// TransactionTableMeta is the shape passed through DataTable's `meta` prop so
// the selection column can do shift-click range select without the column
// definitions owning any state.
export interface TransactionTableMeta {
  onRangeSelect?: (toIndex: number) => void;
  setLastIndex?: (index: number) => void;
}

const selectionColumn: ColumnDef<Transaction> = {
  id: "select",
  meta: { className: "w-px" },
  header: ({ table }) => (
    <Checkbox
      checked={
        table.getIsAllPageRowsSelected() ||
        (table.getIsSomePageRowsSelected() && "indeterminate")
      }
      onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
      aria-label="Select all"
    />
  ),
  cell: ({ row, table }) => {
    const meta = table.options.meta as TransactionTableMeta | undefined;
    return (
      <Checkbox
        checked={row.getIsSelected()}
        // Shift-click extends the selection from the last-toggled row. The
        // normal onCheckedChange still fires and lands the current row in the
        // same (selected) state the range sets, so no preventDefault needed.
        // stopPropagation keeps the row-body select-mode click from also
        // toggling — otherwise a checkbox click would toggle twice.
        onClick={(e) => {
          e.stopPropagation();
          if (e.shiftKey) meta?.onRangeSelect?.(row.index);
        }}
        onCheckedChange={(value) => {
          row.toggleSelected(!!value);
          meta?.setLastIndex?.(row.index);
        }}
        aria-label="Select row"
      />
    );
  },
  enableSorting: false,
};

// Description is the greedy column (no width class); category and amount
// shrink to their content via `w-px` so the description gets the slack and
// its title can truncate.
const baseColumns: ColumnDef<Transaction>[] = [
  {
    id: "description",
    header: "Description",
    // `max-w-0 w-full` is the classic auto-table truncation trick: the cell
    // fills available width but its max-width is 0, so descendants with
    // `truncate` actually clamp. Without this, a long merchant name expands
    // the column and pushes the amount off-screen on narrow viewports.
    meta: { className: "max-w-0 w-full" },
    cell: ({ row }) => <TransactionPrimary transaction={row.original} />,
  },
  {
    id: "category",
    header: () => <div className="text-right">Category</div>,
    // Hidden on mobile — the description column takes the freed space; the
    // category is still editable from the detail page. `text-right` keeps the
    // badge flush against the amount column so the gap is consistent
    // regardless of category name length.
    meta: { className: "w-px hidden sm:table-cell text-right" },
    cell: ({ row }) => (
      <CategoryPicker
        transactionId={row.original.id}
        category={row.original.category}
        overridden={row.original.category_override}
      />
    ),
  },
  {
    accessorKey: "amount",
    header: () => <div className="text-right">Amount</div>,
    // `w-px` shrinks to content on mobile (where space is tight); `sm:min-w-28`
    // pins a comfortable floor on desktop so the right edge of the column
    // stays steady across rows of varying amount length.
    meta: { className: "w-px sm:min-w-28" },
    cell: ({ row }) => <TransactionAmount transaction={row.original} />,
  },
];

// buildTransactionColumns is the single column definition for the v2
// transactions table. Kept out of the route so row rendering is reusable; the
// `selection` option prepends a checkbox column for select mode.
export function buildTransactionColumns(opts?: {
  selection?: boolean;
}): ColumnDef<Transaction>[] {
  return opts?.selection ? [selectionColumn, ...baseColumns] : baseColumns;
}
