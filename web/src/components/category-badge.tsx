import { Badge } from "@/components/ui/badge";
import { DynamicIcon } from "@/lib/icon";
import { cn } from "@/lib/utils";
import type { TransactionCategory } from "@/api/types";

interface CategoryBadgeProps {
  category: TransactionCategory | null | undefined;
  /** Show a subtle ring when the category was set by a manual override. */
  overridden?: boolean;
  /**
   * Visual density.
   * - `sm` — table rows / dense list cells
   * - `md` — default; detail page Hero, pickers, sandbox surfaces
   */
  size?: "sm" | "md";
  className?: string;
}

// Per-size token recipe. Kept inline so the size axis is grep-able alongside
// TagChip's matching recipe — if one shifts the other should too.
const SIZE: Record<"sm" | "md", string> = {
  sm: "h-5 px-1.5 text-[11px] gap-0.5 [&>svg]:size-2.5",
  md: "h-6 px-2 text-xs gap-1 [&>svg]:size-3",
};

// CategoryBadge is the single rendering of a transaction category across v2 —
// list rows, the detail page, pickers. Icon + display name, tinted by the
// category's own color. Rounded-rectangle shaped — the pill shape is reserved
// for tags. Renders an em-dash when there's no category.
export function CategoryBadge({
  category,
  overridden,
  size = "md",
  className,
}: CategoryBadgeProps) {
  if (!category?.display_name) {
    return (
      <span className={cn("text-muted-foreground", className)}>—</span>
    );
  }
  return (
    <Badge
      variant="secondary"
      className={cn(
        "rounded-md",
        SIZE[size],
        overridden && "ring-1 ring-primary/40",
        className,
      )}
    >
      {category.icon && (
        <DynamicIcon
          name={category.icon}
          style={category.color ? { color: category.color } : undefined}
        />
      )}
      {category.display_name}
    </Badge>
  );
}
