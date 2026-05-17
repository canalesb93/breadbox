import {
  ColorRailCardSkeleton,
  type ColorRailCardSkeletonProps,
} from "@/components/color-rail-card";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

interface DetailPageSkeletonProps {
  /**
   * Hero options forwarded to `<ColorRailCardSkeleton>`. The hero is
   * always rendered — every v2 detail page leads with a hero card, so
   * the contract is "you must mirror the loaded hero shape".
   */
  hero?: Pick<ColorRailCardSkeletonProps, "tileShape" | "withFooter" | "body">;
  /**
   * Number of `<JumpToPill>`-shaped pills to render below the hero
   * (matches the iter-75 `<JumpToRow>` vocabulary — `h-7` pills).
   * Omit (or pass `0`) when the detail page doesn't carry a lateral-nav
   * pill strip (e.g. Connection detail puts its actions in the hero
   * footer instead).
   */
  jumpPills?: number;
  /**
   * Heights for the main column's stacked block placeholders, in
   * Tailwind class form (e.g. `["h-72"]`, `["h-32", "h-48", "h-48"]`).
   * Each entry renders as a `rounded-xl` skeleton block — matches the
   * `<SectionCard>` / `<ListCard>` chrome the loaded page renders.
   */
  main?: ReadonlyArray<string>;
  /**
   * Heights for the sidebar column's stacked block placeholders, in
   * Tailwind class form (e.g. `["h-56"]`, `["h-40", "h-48"]`). When
   * the loaded page has no sidebar (rare), pass an empty array — the
   * grid collapses to a single column.
   */
  sidebar?: ReadonlyArray<string>;
  className?: string;
}

// DetailPageSkeleton is the canonical loading shell for every v2 detail
// page. It composes a `<ColorRailCardSkeleton>` hero with a
// `<JumpToRow>`-shaped pill strip and a two-column grid of `rounded-xl`
// block placeholders matching `<SectionCard>` / `<ListCard>` chrome.
//
// Three loading vocabularies, one visual system: error → `<PageError>`,
// loading → `<DetailPageSkeleton>`, empty → `<EmptyState>`. Don't fork —
// extend this primitive if a new layout knob is needed.
export function DetailPageSkeleton({
  hero,
  jumpPills = 0,
  main = [],
  sidebar = [],
  className,
}: DetailPageSkeletonProps) {
  const hasSidebar = sidebar.length > 0;

  return (
    <div className={cn("space-y-6", className)}>
      <ColorRailCardSkeleton
        tileShape={hero?.tileShape}
        withFooter={hero?.withFooter}
        body={hero?.body}
      />
      {jumpPills > 0 && (
        <div className="flex gap-2">
          {Array.from({ length: jumpPills }).map((_, i) => (
            <Skeleton key={i} className="h-7 w-32 rounded-md" />
          ))}
        </div>
      )}
      {(main.length > 0 || hasSidebar) && (
        <div
          className={cn(
            "grid gap-6",
            hasSidebar && "lg:grid-cols-[minmax(0,1fr)_18rem]",
          )}
        >
          {main.length > 0 && (
            <div className="min-w-0 space-y-6">
              {main.map((h, i) => (
                <Skeleton key={i} className={cn(h, "rounded-xl")} />
              ))}
            </div>
          )}
          {hasSidebar && (
            <div className="space-y-6">
              {sidebar.map((h, i) => (
                <Skeleton key={i} className={cn(h, "rounded-xl")} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
