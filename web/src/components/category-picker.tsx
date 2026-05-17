import { useState } from "react";
import { Pencil, Plus } from "lucide-react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { CategoryBadge } from "@/components/category-badge";
import {
  CategoryCommandList,
  useCategoryEditor,
  type CategoryPick,
} from "@/components/category-command";
import { cn } from "@/lib/utils";
import type { TransactionCategory } from "@/api/types";

interface CategoryPickerProps {
  transactionId: string;
  category: TransactionCategory | null | undefined;
  /** True when the current category was set by a manual override. */
  overridden?: boolean;
  /**
   * Badge density. Defaults to `md` (h-6) — the picker only renders inside
   * the transactions row's category column which is `hidden sm:table-cell`,
   * so there's room for the comfier size. Pass `sm` for tighter rails.
   */
  size?: "sm" | "md";
  /**
   * Override the default behaviour (single-tx update mutation + toast).
   * Used by the sandbox specimen and any future caller that wants to
   * own the pick handler (bulk flows, dry-run previews, etc.).
   */
  onPick?: (pick: CategoryPick) => void | Promise<void>;
  className?: string;
}

// CategoryPicker is the compact, inline category picker used in transaction
// rows — a rounded-rect trigger showing the current category (or an "add"
// affordance when uncategorized) over a searchable popover. stopPropagation
// keeps a click from also firing the row's navigate handler.
export function CategoryPicker({
  transactionId,
  category,
  overridden,
  size = "md",
  onPick: onPickOverride,
  className,
}: CategoryPickerProps) {
  // Match the empty-state pill's geometry to the active badge so the column
  // doesn't shift when the user assigns or clears a category from the row.
  const emptyPillClass =
    size === "sm"
      ? "h-5 px-1.5 text-[11px] gap-0.5"
      : "h-6 px-2 text-xs gap-1";
  const [open, setOpen] = useState(false);
  const { apply, isPending } = useCategoryEditor(transactionId);

  const onPick = async (pick: CategoryPick) => {
    setOpen(false);
    if (onPickOverride) {
      await onPickOverride(pick);
      return;
    }
    await apply(pick);
  };

  // Chevron swaps in for the category icon on hover (cross-fade in the
  // same spot) so the badge surface stays the same width and nothing
  // overlaps the label. Falls back to a no-op when the category has no
  // icon — the hover ring alone carries the affordance there.
  const chevronClass = size === "sm" ? "size-2.5" : "size-3";
  // Mask is intentionally bigger than the icon — and wider than tall —
  // so stroke/edge bleed and per-icon viewBox variance on the underlying
  // svg are fully covered (some lucide icons render a px or two past
  // their nominal width).
  const maskClass = size === "sm" ? "h-3 w-4" : "h-4 w-5";
  const chevronLeftClass = size === "sm" ? "left-1" : "left-1";
  const hasIcon = !!category?.icon;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          disabled={isPending}
          onClick={(e) => e.stopPropagation()}
          className={cn(
            "group/picker focus-visible:ring-ring relative inline-flex items-center rounded-md transition-shadow focus-visible:ring-2 focus-visible:outline-none disabled:cursor-wait disabled:opacity-50",
            // Hover ring only when a category is assigned. Empty-state
            // pill keeps its dashed border alone (no double-stroke).
            category?.display_name && "hover:ring-1 hover:ring-border",
            className,
          )}
        >
          {category?.display_name ? (
            <CategoryBadge
              category={category}
              overridden={overridden}
              size={size}
            />
          ) : (
            <span
              className={cn(
                "text-muted-foreground border-border inline-flex items-center rounded-md border border-dashed group-hover/picker:text-foreground group-hover/picker:border-foreground/40",
                emptyPillClass,
              )}
            >
              <Plus className={size === "sm" ? "size-2.5" : "size-3"} />
              Category
            </span>
          )}
          {/* Pencil swap-in: overlays the category icon's spot on hover.
              The wrapper carries a `bg-secondary` fill (matching the
              CategoryBadge surface) so it masks the underlying icon —
              cleaner than trying to fade the badge's own svg via a
              descendant selector. */}
          {hasIcon && (
            <span
              aria-hidden
              className={cn(
                "bg-secondary pointer-events-none absolute top-1/2 flex -translate-y-1/2 items-center justify-center rounded-[2px] opacity-0 transition-opacity group-hover/picker:opacity-100 group-focus-visible/picker:opacity-100",
                chevronLeftClass,
                maskClass,
              )}
            >
              <Pencil className={cn("text-muted-foreground", chevronClass)} />
            </span>
          )}
        </button>
      </PopoverTrigger>
      <PopoverContent
        className="w-64 p-0"
        align="start"
        onClick={(e) => e.stopPropagation()}
      >
        <CategoryCommandList
          currentSlug={category?.slug}
          showReset={overridden}
          onPick={onPick}
        />
      </PopoverContent>
    </Popover>
  );
}
