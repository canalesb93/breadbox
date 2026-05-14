import { useState } from "react";
import { Plus } from "lucide-react";
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
  className,
}: CategoryPickerProps) {
  const [open, setOpen] = useState(false);
  const { apply, isPending } = useCategoryEditor(transactionId);

  const onPick = async (pick: CategoryPick) => {
    setOpen(false);
    await apply(pick);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          disabled={isPending}
          onClick={(e) => e.stopPropagation()}
          className={cn(
            "hover:bg-accent focus-visible:ring-ring rounded-md transition-colors focus-visible:ring-2 focus-visible:outline-none disabled:opacity-50",
            className,
          )}
        >
          {category?.display_name ? (
            <CategoryBadge category={category} overridden={overridden} />
          ) : (
            <span className="text-muted-foreground border-border hover:text-foreground inline-flex items-center gap-1 rounded-md border border-dashed px-2 py-0.5 text-xs">
              <Plus className="size-3" />
              Category
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
