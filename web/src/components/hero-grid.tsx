import * as React from "react";
import { cn } from "@/lib/utils";

interface HeroGridProps extends React.HTMLAttributes<HTMLDivElement> {
  /**
   * Override the row-gap on lg+. Defaults to `lg:gap-10` (the shared
   * `gap-10` axis used by account / category / connection detail heroes).
   * Transaction-detail uses `lg:gap-x-10 lg:gap-y-5` because the left
   * column stacks an identity row on top of a classify row and the
   * vertical rhythm needs to be tighter — pass `"lg:gap-x-10 lg:gap-y-5"`
   * to opt in.
   */
  lgGapClassName?: string;
  children: React.ReactNode;
}

// HeroGrid is the canonical body grid inside a detail-page hero card —
// the layer that sits one level inside `<ColorRailCard>` and arranges
// the identity column on the left and the metric column on the right.
// Promoted in iter 97 from three byte-identical sites (account-detail,
// category-detail, connection-detail) plus a near-identical transaction-
// detail variant. 25th shared primitive in the v2 vocabulary.
//
// Visual contract:
//   `grid gap-5 px-5 py-5 sm:gap-6 sm:px-7 sm:py-6
//    lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start lg:gap-10`
//
// The grid breakpoints encode the same density story across every
// detail page: mobile stacks rows top-to-bottom (identity → metric →
// optional extras); ≥640px loosens spacing without changing layout;
// ≥1024px docks the metric column to the right edge so the identity
// column gets the priority left slot. Always render inside
// `<ColorRailCard>` — HeroGrid is the body layer, not the chrome.
export function HeroGrid({
  className,
  lgGapClassName = "lg:gap-10",
  children,
  ...rest
}: HeroGridProps) {
  return (
    <div
      className={cn(
        "grid gap-5 px-5 py-5 sm:gap-6 sm:px-7 sm:py-6 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start",
        lgGapClassName,
        className,
      )}
      {...rest}
    >
      {children}
    </div>
  );
}
