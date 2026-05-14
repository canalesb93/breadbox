import { Badge } from "@/components/ui/badge";
import { DynamicIcon } from "@/lib/icon";
import { cn } from "@/lib/utils";
import type { TransactionCategory } from "@/api/types";

interface CategoryBadgeProps {
  category: TransactionCategory | null | undefined;
  /** Show a subtle ring when the category was set by a manual override. */
  overridden?: boolean;
  className?: string;
}

// CategoryBadge is the single rendering of a transaction category across v2 —
// list rows, the detail page, pickers. Icon + display name, tinted by the
// category's own color. Rounded-rectangle shaped — the pill shape is reserved
// for tags. Renders an em-dash when there's no category.
export function CategoryBadge({
  category,
  overridden,
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
        "gap-1 rounded-md",
        overridden && "ring-1 ring-primary/40",
        className,
      )}
    >
      {category.icon && (
        <DynamicIcon
          name={category.icon}
          className="size-3"
          style={category.color ? { color: category.color } : undefined}
        />
      )}
      {category.display_name}
    </Badge>
  );
}
