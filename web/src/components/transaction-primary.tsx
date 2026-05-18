import { Tag as TagIcon } from "lucide-react";
import { CategoryIconTile } from "@/components/category-icon-tile";
import { TagList } from "@/components/tag-chip";
import { cn } from "@/lib/utils";
import type { Transaction } from "@/api/types";

interface TransactionPrimaryProps {
  transaction: Transaction;
  /**
   * When provided, the merchant title becomes a click target that fires
   * this callback (and stops propagation so the surrounding row click
   * — typically focus / enter-select — doesn't also fire). Used by the
   * transactions list row to open the detail page from the title only.
   */
  onTitleClick?: (transaction: Transaction) => void;
  className?: string;
}

// Identity block for a transaction: category icon tile, bank description
// with an inline muted `Pending` label, and a secondary line of metadata
// (account · member · tags). Pending is plain text — it's the only
// secondary signal in the primary line and the surrounding row already
// hosts pill-shaped tags + category badges, so an additional pill here
// just adds visual noise.
export function TransactionPrimary({
  transaction: t,
  onTitleClick,
  className,
}: TransactionPrimaryProps) {
  const titleClasses = "truncate font-medium";
  const title = onTitleClick ? (
    <button
      type="button"
      onClick={(e) => {
        e.stopPropagation();
        onTitleClick(t);
      }}
      // Subtle navigation hint: title underlines on its own hover AND when
      // the surrounding row is hovered (the whole row is a click target
      // that navigates to detail, so the title is the visual anchor for
      // that affordance). `group-hover/row:` is set up on TableRow.
      className={cn(
        titleClasses,
        "cursor-pointer underline-offset-2 decoration-muted-foreground/50 hover:underline group-hover/row:underline focus-visible:outline-none focus-visible:underline text-left",
      )}
    >
      {t.provider_name}
    </button>
  ) : (
    <span className={titleClasses}>{t.provider_name}</span>
  );
  return (
    <div className={cn("flex min-w-0 items-center gap-3", className)}>
      <CategoryIconTile
        icon={t.category?.icon}
        color={t.category?.color}
        size="sm"
      />
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          {title}
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

// On sm+ this is "account · user · <tag chips>". On mobile we drop the
// household member and collapse the tag chips into a compact "N tag-icon" so
// the row stays narrow enough to keep the amount visible without horizontal
// scroll.
function TransactionMeta({ transaction: t }: { transaction: Transaction }) {
  const hasTags = !!t.tags?.length;
  if (!t.account_name && !t.user_name && !hasTags) return null;

  return (
    <div className="text-muted-foreground mt-0.5 flex min-w-0 items-center gap-1.5 text-xs">
      {t.account_name && <span className="truncate">{t.account_name}</span>}
      {t.user_name && (
        <span className="hidden truncate sm:inline">· {t.user_name}</span>
      )}
      {hasTags && (
        <>
          <span aria-hidden className="hidden sm:inline">
            ·
          </span>
          <span className="inline-flex shrink-0 items-center gap-0.5 sm:hidden">
            {t.account_name && <span aria-hidden>·</span>}
            <TagIcon className="size-3" />
            {t.tags!.length}
          </span>
          <span className="hidden sm:inline-flex">
            <TagList slugs={t.tags} max={2} size="xs" />
          </span>
        </>
      )}
    </div>
  );
}
