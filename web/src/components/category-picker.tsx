import { useState } from "react";
import { ChevronDown, Plus } from "lucide-react";
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
    await apply(pick);
  };

  // `group/picker` lets the chevron hint reveal only on hover/focus
  // without taking layout space at rest, so a tx row's category column
  // reads as a tidy badge until the user shows intent to edit.
  const chevronClass =
    size === "sm" ? "size-2.5 -mr-0.5" : "size-3 -mr-0.5";

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          disabled={isPending}
          onClick={(e) => e.stopPropagation()}
          className={cn(
            // Pickier hover state — bg + ring — than the previous
            // `hover:bg-accent` alone, so the affordance reads at-a-glance
            // even when the badge already carries its own coloured tint.
            "group/picker focus-visible:ring-ring inline-flex items-center gap-1 rounded-md p-0.5 transition-colors hover:bg-accent hover:ring-1 hover:ring-border focus-visible:ring-2 focus-visible:outline-none disabled:cursor-wait disabled:opacity-50",
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
          <ChevronDown
            aria-hidden
            className={cn(
              // Hidden at rest, revealed on group hover or keyboard
              // focus so the picker affordance is discoverable without
              // adding weight to the badge.
              "text-muted-foreground opacity-0 transition-opacity group-hover/picker:opacity-100 group-focus-visible/picker:opacity-100",
              chevronClass,
            )}
          />
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
