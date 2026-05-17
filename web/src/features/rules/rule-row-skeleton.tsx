import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

interface RuleRowSkeletonProps {
  /**
   * Per-row width override for the title placeholder. The defaults rotate
   * across `Array.from(...).map(i, …)` calls so the loading stack reads as
   * a stack of varied-length rules instead of a metronome.
   */
  titleClassName?: string;
  /** Whether to render the lg-only stats columns (stage + last active). */
  showStats?: boolean;
  className?: string;
}

/**
 * RuleRowSkeleton mirrors the layout of `<RuleRow>` while the rules list
 * loads — same `rounded-xl border` card, same `size-9 rounded-xl` avatar
 * tile, same title + meta-line stack, and the lg-only stage / last-active
 * columns + the absolute action cluster on the right. Lifts the rules
 * list out of the generic `Skeleton h-[72px] rounded-xl` block strip and
 * onto a real row-shape skeleton, matching the polish the rest of the v2
 * list pages (Connections / Accounts / Categories / Transactions) ship.
 */
export function RuleRowSkeleton({
  titleClassName = "w-48",
  showStats = true,
  className,
}: RuleRowSkeletonProps) {
  return (
    <div
      className={cn(
        "bg-card relative rounded-xl border",
        className,
      )}
    >
      <div className="flex items-center gap-3 px-4 py-4 pr-20 sm:gap-4 sm:px-5 sm:py-5 sm:pr-24">
        <Skeleton className="size-9 shrink-0 rounded-xl" />
        <div className="min-w-0 flex-1 space-y-2">
          <Skeleton className={cn("h-3.5", titleClassName)} />
          <div className="flex items-center gap-2">
            <Skeleton className="h-3 w-14" />
            <span className="text-muted-foreground/40 text-xs">·</span>
            <Skeleton className="h-3 w-16" />
            <span className="text-muted-foreground/40 text-xs">·</span>
            <Skeleton className="h-3 w-12" />
          </div>
        </div>
        {showStats && (
          <div className="hidden shrink-0 items-center gap-4 lg:flex">
            <div className="flex w-16 flex-col items-center gap-1">
              <Skeleton className="h-3.5 w-10" />
              <Skeleton className="h-2.5 w-8" />
            </div>
            <div className="flex w-20 flex-col items-center gap-1">
              <Skeleton className="h-3 w-12" />
              <Skeleton className="h-2.5 w-14" />
            </div>
          </div>
        )}
      </div>
      <div className="absolute top-1/2 right-3 flex -translate-y-1/2 items-center gap-0.5">
        <Skeleton className="size-7 rounded-md" />
        <Skeleton className="size-7 rounded-md" />
      </div>
    </div>
  );
}
