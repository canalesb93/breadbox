import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

export type EmptyStateVariant = "default" | "card" | "inline";

export interface EmptyStateProps {
  icon?: LucideIcon;
  title: string;
  description?: string;
  /** Optional call-to-action — usually a <Button>. */
  action?: React.ReactNode;
  /**
   * `default` — bare centered block. Reach for this inside an already-bordered
   * container (table emptyState slot, ListCard's `empty` slot, SectionCard
   * body).
   *
   * `card` — adds a dashed bordered card around the block. Reach for this
   * when the empty state lives in raw page space (settings section, sub-panel
   * that doesn't carry its own card wrapper) so it reads as a placeholder
   * waiting to be filled instead of floating text.
   *
   * `inline` — tighter centered block with a smaller icon tile. Use for
   * compact secondary panels where the full empty-state weight would be too
   * loud (a sync history rail inside a section, or the right-column accounts
   * list on connection-detail).
   */
  variant?: EmptyStateVariant;
  className?: string;
}

// Zero-data state for a page or section that loaded fine but has nothing to
// show. Distinct from routes/placeholder.tsx, which marks a page that hasn't
// been built yet.
//
// Visual contract:
// - Always centered, icon tile → title → description → action.
// - Icon tile is a soft muted square (`rounded-xl`), never a circle — matches
//   the rest of the v2 icon language (see ColorRailCard, StatusPanel).
// - Title is `text-sm font-medium`; description is `text-sm text-muted-foreground`
//   so an empty state nested inside a card body doesn't dominate.
export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  variant = "default",
  className,
}: EmptyStateProps) {
  const tileSize = variant === "inline" ? "size-9" : "size-11";
  const iconSize = variant === "inline" ? "size-4" : "size-5";
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center gap-2 text-center",
        variant === "default" && "py-12",
        variant === "card" &&
          "border-border/70 bg-muted/10 rounded-lg border border-dashed px-6 py-10",
        variant === "inline" && "py-8",
        className,
      )}
    >
      {Icon && (
        <div
          className={cn(
            "bg-muted text-muted-foreground mb-2 flex items-center justify-center rounded-xl",
            tileSize,
          )}
        >
          <Icon className={iconSize} />
        </div>
      )}
      <h3 className="text-sm font-medium">{title}</h3>
      {description && (
        <p className="text-muted-foreground max-w-sm text-sm">{description}</p>
      )}
      {action && <div className="mt-3">{action}</div>}
    </div>
  );
}
