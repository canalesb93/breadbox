import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

// ListRowSkeleton is the canonical loading-row shape used by every v2 list
// surface. It mirrors the layout of the real row underneath (icon tile +
// title/subtitle stack + optional trailing block) so the table doesn't shift
// when data arrives.
//
// Density tokens encode the real row's vertical rhythm:
//   - "compact"  → px-4 py-2.5 gap-3  (Categories tree)
//   - "regular"  → px-5 py-3   gap-3  (Home recent activity, Home connections)
//   - "comfortable" → px-5 py-3.5 gap-3 sm:gap-4 (Connections + Accounts list pages)
//
// `leading` controls the icon-tile size + shape:
//   - "sm-square" → size-7 rounded-md  (Home recent activity — matches
//                                       TransactionPrimary's CategoryIconTile)
//   - "md-square" → size-9 rounded-md  (Connections, Categories, Home connections)
//   - "lg-square" → size-10 rounded-lg (Accounts)
//
// `trailing` controls the right-hand block:
//   - "none" → no trailing element
//   - "badge" → single ~h-5 chip
//   - "value-stack" → 2-line right-aligned stack (amount / sub-label)
//
// `lines` toggles single-line vs two-line title stack. Every consumer that
// renders a "subtitle" under the title should pick `lines: 2` so widths stay
// realistic during load.
type Density = "compact" | "regular" | "comfortable";
type Leading = "sm-square" | "md-square" | "lg-square";
type Trailing = "none" | "badge" | "value-stack";

interface ListRowSkeletonProps {
  density?: Density;
  leading?: Leading;
  lines?: 1 | 2;
  trailing?: Trailing;
  /**
   * Override widths to vary the visual rhythm row-to-row. The defaults are
   * tuned to look natural for typical list copy (title 32-40, subtitle 22-28).
   */
  titleClassName?: string;
  subtitleClassName?: string;
  trailingTopClassName?: string;
  trailingBottomClassName?: string;
  className?: string;
}

const DENSITY: Record<Density, string> = {
  compact: "px-4 py-2.5 gap-3",
  regular: "px-5 py-3 gap-3",
  comfortable: "px-5 py-3.5 gap-3 sm:gap-4",
};

const LEADING: Record<Leading, string> = {
  "sm-square": "size-7 rounded-md",
  "md-square": "size-9 rounded-md",
  "lg-square": "size-10 rounded-lg",
};

export function ListRowSkeleton({
  density = "regular",
  leading = "md-square",
  lines = 2,
  trailing = "none",
  titleClassName = "w-36",
  subtitleClassName = "w-24",
  trailingTopClassName = "w-16",
  trailingBottomClassName = "w-10",
  className,
}: ListRowSkeletonProps) {
  return (
    <div className={cn("flex items-center", DENSITY[density], className)}>
      <Skeleton className={cn("shrink-0", LEADING[leading])} />
      <div className="min-w-0 flex-1 space-y-1.5">
        <Skeleton className={cn("h-3.5", titleClassName)} />
        {lines === 2 && (
          <Skeleton className={cn("h-3", subtitleClassName)} />
        )}
      </div>
      {trailing === "badge" && (
        <Skeleton className={cn("h-5 rounded-md", trailingTopClassName)} />
      )}
      {trailing === "value-stack" && (
        <div className="space-y-1.5 text-right">
          <Skeleton className={cn("ml-auto h-3.5", trailingTopClassName)} />
          <Skeleton className={cn("ml-auto h-3", trailingBottomClassName)} />
        </div>
      )}
    </div>
  );
}
