import * as React from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

interface ColorRailCardProps extends React.HTMLAttributes<HTMLDivElement> {
  /**
   * @deprecated The 4px coloured left rail was retired during the
   * design-system polish pass — the eyebrow + icon tile carry enough of
   * the meaning the rail used to encode, and a flat-edge hero card sits
   * more quietly next to surrounding sections. Prop is kept as a no-op
   * so callers don't have to be touched in the same change; clean up in
   * a follow-up.
   */
  accent?: string | null;
  /** Optional class applied to the bordered card wrapper. */
  cardClassName?: string;
  /**
   * Optional footer slot rendered with a top border + tinted muted
   * background — matches the inline action strip used on the
   * transaction- and account-detail heroes.
   */
  footer?: React.ReactNode;
  /** Optional class applied to the footer wrapper. */
  footerClassName?: string;
  children: React.ReactNode;
}

// ColorRailCard is the canonical "detail-page hero card" container — a
// bordered card that frames the hero content on transaction, account,
// category, and connection detail pages plus the home stats and provider
// tiles. The original implementation carried a 4px coloured left rail
// (category color, accounting role, …) which was retired during a
// design-system polish pass; the eyebrow + icon tile already carry the
// meaning. Name kept because consumers reference it across the codebase
// — rename in a follow-up if the rail-less treatment sticks.
//
// Visual contract:
//   `bg-card overflow-hidden rounded-xl border`
//     children
//     optional `<div className="border-t bg-muted/20 ...">footer</div>`
export function ColorRailCard({
  accent: _accent,
  cardClassName,
  className,
  footer,
  footerClassName,
  children,
  ...rest
}: ColorRailCardProps) {
  return (
    <div
      className={cn(
        "bg-card overflow-hidden rounded-xl border",
        cardClassName,
        className,
      )}
      {...rest}
    >
      {children}
      {footer && (
        <div
          className={cn(
            "border-t bg-muted/20 flex flex-wrap items-center justify-end gap-2 px-5 py-3 sm:px-7",
            footerClassName,
          )}
        >
          {footer}
        </div>
      )}
    </div>
  );
}

export interface ColorRailCardSkeletonProps {
  /**
   * Shape of the hero icon tile to mirror — match what the loaded hero
   * renders. `rounded-md` for transaction-detail (matches
   * `<CategoryIconTile>`), `rounded-lg` for account-detail and
   * category-detail (slightly chunkier accounting/folder tile).
   */
  tileShape?: "rounded-md" | "rounded-lg";
  /**
   * Whether to render the bordered footer strip (action bar) under the
   * hero. Matches `<ColorRailCard footer={…}>` consumers.
   */
  withFooter?: boolean;
  /**
   * Optional content slot rendered between the hero body and the footer
   * strip — used by transaction-detail for the secondary details grid
   * that sits below the identity row.
   */
  body?: React.ReactNode;
  className?: string;
}

// ColorRailCardSkeleton mirrors the `<ColorRailCard>` wrapper for loading
// states — the same `bg-card rounded-xl border` shell. The identity
// column on the left is a stable shape across every detail-page hero
// (size-12 tile, eyebrow line, title line, meta line) so it lives inside
// the primitive; the trailing column carries the per-entity metric
// (amount / balance / count) and is a smaller shared shape. Don't fork
// the look — consumers can hang an additional `body` row off the bottom
// (e.g. TX-detail's secondary details grid) or opt in to a `withFooter`
// strip when the loaded hero has a footer action bar.
export function ColorRailCardSkeleton({
  tileShape = "rounded-md",
  withFooter = false,
  body,
  className,
}: ColorRailCardSkeletonProps) {
  return (
    <div
      className={cn(
        "bg-card overflow-hidden rounded-xl border",
        className,
      )}
    >
      <div className="grid gap-5 px-5 py-5 sm:gap-6 sm:px-7 sm:py-6 lg:grid-cols-[minmax(0,1fr)_auto]">
        {/* Identity column */}
        <div className="flex items-start gap-3 sm:gap-4">
          <Skeleton className={cn("size-12", tileShape)} />
          <div className="space-y-2 py-1">
            <Skeleton className="h-3 w-20" />
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-3 w-48" />
          </div>
        </div>
        {/* Metric column */}
        <div className="space-y-2 lg:items-end">
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-9 w-32" />
          <Skeleton className="h-3 w-28" />
        </div>
        {body && <div className="lg:col-span-2">{body}</div>}
      </div>
      {withFooter && (
        <div className="border-t flex justify-end gap-2 px-5 py-3 sm:px-7">
          <Skeleton className="h-7 w-24 rounded-md" />
          <Skeleton className="h-7 w-32 rounded-md" />
        </div>
      )}
    </div>
  );
}
