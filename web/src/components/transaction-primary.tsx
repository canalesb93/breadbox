import { CategoryIconTile } from "@/components/category-icon-tile";
import { TagList } from "@/components/tag-chip";
import { cn } from "@/lib/utils";
import type { Transaction } from "@/api/types";

interface TransactionPrimaryProps {
  transaction: Transaction;
  className?: string;
}

// TransactionPrimary is the reusable identity block for a transaction — the
// category icon tile, the bank description as the title with an inline
// "Pending" marker, and a secondary line of metadata (account · member · tags).
// Shared by the transactions table and any future transaction list.
export function TransactionPrimary({
  transaction: t,
  className,
}: TransactionPrimaryProps) {
  return (
    <div className={cn("flex items-center gap-3", className)}>
      <CategoryIconTile
        icon={t.category?.icon}
        color={t.category?.color}
        size="sm"
      />
      <div className="min-w-0">
        <div className="flex items-center gap-1.5">
          <span className="truncate font-medium">{t.provider_name}</span>
          {t.pending && (
            <span className="text-muted-foreground shrink-0 text-xs">
              Pending
            </span>
          )}
        </div>
        <TransactionMeta transaction={t} />
      </div>
    </div>
  );
}

// The secondary metadata line: account and household member as plain text,
// then tag chips. Renders nothing when there's no metadata to show.
function TransactionMeta({ transaction: t }: { transaction: Transaction }) {
  const text: string[] = [];
  if (t.account_name) text.push(t.account_name);
  if (t.user_name) text.push(t.user_name);
  const hasTags = !!t.tags?.length;
  if (!text.length && !hasTags) return null;

  return (
    <div className="text-muted-foreground mt-0.5 flex items-center gap-1.5 text-xs">
      {text.length > 0 && <span className="truncate">{text.join(" · ")}</span>}
      {text.length > 0 && hasTags && <span aria-hidden>·</span>}
      {hasTags && <TagList slugs={t.tags} max={2} />}
    </div>
  );
}
