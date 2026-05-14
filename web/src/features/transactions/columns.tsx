import type { ColumnDef } from "@tanstack/react-table";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { CategoryBadge } from "@/components/category-badge";
import { CategoryIconTile } from "@/components/category-icon-tile";
import { TagList } from "@/components/tag-chip";
import { formatAmount, formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";
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
        onClick={(e) => {
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

const baseColumns: ColumnDef<Transaction>[] = [
  {
    accessorKey: "date",
    header: "Date",
    cell: ({ row }) => (
      <span className="text-muted-foreground tabular-nums whitespace-nowrap">
        {formatDate(row.original.date)}
      </span>
    ),
  },
  {
    id: "description",
    header: "Description",
    cell: ({ row }) => {
      const t = row.original;
      // provider_name is the raw bank description (the primary label, per the
      // #1072 decision). provider_merchant_name is the cleaned merchant — show
      // it as a subtitle only when it adds information.
      const subtitle =
        t.provider_merchant_name &&
        t.provider_merchant_name !== t.provider_name
          ? t.provider_merchant_name
          : null;
      return (
        <div className="flex items-center gap-3">
          <CategoryIconTile
            icon={t.category?.icon}
            color={t.category?.color}
            size="sm"
          />
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="truncate font-medium">{t.provider_name}</span>
              {t.pending && (
                <Badge
                  variant="outline"
                  className="text-muted-foreground shrink-0"
                >
                  Pending
                </Badge>
              )}
            </div>
            {subtitle && (
              <div className="text-muted-foreground truncate text-xs">
                {subtitle}
              </div>
            )}
            <TagList slugs={t.tags} max={3} className="mt-1" />
          </div>
        </div>
      );
    },
  },
  {
    id: "category",
    header: "Category",
    cell: ({ row }) => (
      <CategoryBadge
        category={row.original.category}
        overridden={row.original.category_override}
      />
    ),
  },
  {
    accessorKey: "account_name",
    header: "Account",
    cell: ({ row }) => (
      <span className="text-muted-foreground whitespace-nowrap">
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
          className={cn(
            "text-right font-medium tabular-nums whitespace-nowrap",
            isInflow && "text-emerald-600 dark:text-emerald-500",
          )}
        >
          {formatAmount(t.amount, t.iso_currency_code)}
        </div>
      );
    },
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
