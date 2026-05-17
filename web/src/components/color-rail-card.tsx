import * as React from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

interface ColorRailCardProps extends React.HTMLAttributes<HTMLDivElement> {
  /**
   * CSS color value for the 4px left rail. Pass a CSS variable
   * (`"var(--destructive)"`) or a literal (`"#f97316"`). When omitted
   * the rail collapses to `--muted` so the card still reads as a hero,
   * but the colour stops carrying meaning.
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
// bordered card with a 4px coloured left rail that encodes meaning
// (category color for transactions, accounting role for accounts,
// category's own color on the category-detail page). Sibling of
// `<SectionCard>` and `<ListCard>`; this is the third primitive in the
// v2 design vocabulary established by iter 5 (TX-detail hero) and iter 6
// (Account-detail hero, where the iter-5/6 drift note explicitly called
// for extraction once a third surface adopts it).
//
// Visual contract:
//   `bg-card relative overflow-hidden rounded-xl border`
//     `<div aria-hidden className="absolute inset-y-0 left-0 w-1" />`  ← rail
//     children
//     optional `<div className="border-t bg-muted/20 ...">footer</div>`
//
// The colour-rail principle: the rail's tint encodes *meaning* (asset vs
// liability, this transaction's category, this category's own colour),
// not decoration. Excluded / muted states collapse to `--muted` so the
// card reads "shelved" rather than "demands attention". Always pair the
// rail with a small uppercase eyebrow so colour alone never carries the
// signal.
export function ColorRailCard({
  accent,
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
        "bg-card relative overflow-hidden rounded-xl border",
        cardClassName,
        className,
      )}
      {...rest}
    >
      <div
        aria-hidden
        className="absolute inset-y-0 left-0 w-1"
        style={{ backgroundColor: accent ?? "var(--muted)" }}
      />
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
// states — the same `bg-card rounded-xl border` shell + 4px muted left
// rail. The identity column on the left is a stable shape across every
// detail-page hero (size-12 tile, eyebrow line, title line, meta line) so
// it lives inside the primitive; the trailing column carries the
// per-entity metric (amount / balance / count) and is a smaller shared
// shape. Don't fork the look — consumers can hang an additional `body`
// row off the bottom (e.g. TX-detail's secondary details grid) or opt in
// to a `withFooter` strip when the loaded hero has a footer action bar.
export function ColorRailCardSkeleton({
  tileShape = "rounded-md",
  withFooter = false,
  body,
  className,
}: ColorRailCardSkeletonProps) {
  return (
    <div
      className={cn(
        "bg-card relative overflow-hidden rounded-xl border",
        className,
      )}
    >
      <div
        aria-hidden
        className="bg-muted absolute inset-y-0 left-0 w-1"
      />
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
