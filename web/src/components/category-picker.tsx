import { useState } from "react";
import { Pencil, Plus } from "lucide-react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  CATEGORY_BADGE_ICON,
  CATEGORY_BADGE_SHAPE,
  CategoryBadge,
} from "@/components/category-badge";
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
  const [open, setOpen] = useState(false);
  const { apply, isPending } = useCategoryEditor(transactionId);

  const onPick = async (pick: CategoryPick) => {
    setOpen(false);
    await (onPickOverride ?? apply)(pick);
  };

  const iconClass = CATEGORY_BADGE_ICON[size];
  // Width tuned to end at the badge's text edge — wider eats into the
  // label, narrower leaves the icon's right-side stroke peeking through.
  const maskClass = size === "sm" ? "h-3 w-3.5" : "h-4 w-5";
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
            // Empty-state pill carries its own dashed border — no double-stroke.
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
                CATEGORY_BADGE_SHAPE[size],
              )}
            >
              <Plus className={iconClass} />
              Category
            </span>
          )}
          {hasIcon && (
            <span
              aria-hidden
              className={cn(
                "bg-secondary pointer-events-none absolute top-1/2 left-1 flex -translate-y-1/2 items-center justify-center rounded-[2px] opacity-0 transition-opacity group-hover/picker:opacity-100 group-focus-visible/picker:opacity-100",
                maskClass,
              )}
            >
              <Pencil className={cn("text-muted-foreground", iconClass)} />
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
