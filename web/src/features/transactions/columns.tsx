import type { ColumnDef } from "@tanstack/react-table";
import { Badge } from "@/components/ui/badge";
import { CategoryBadge } from "@/components/category-badge";
import { CategoryIconTile } from "@/components/category-icon-tile";
import { TagList } from "@/components/tag-chip";
import { formatAmount, formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Transaction } from "@/api/types";

// transactionColumns is the single column definition for the v2 transactions
// table — kept out of the route so the row rendering is reusable and the
// route file stays focused on data + URL state. Sorting, selection, and other
// per-context columns are layered on by later PRs in this stack.
export const transactionColumns: ColumnDef<Transaction>[] = [
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
