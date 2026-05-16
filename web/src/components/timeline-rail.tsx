import * as React from "react";
import { type LucideIcon } from "lucide-react";
import { Eyebrow } from "@/components/eyebrow";
import { cn } from "@/lib/utils";

// TimelineRail is the v2 vocabulary for a vertical activity feed: a thin
// `border-l` rail anchors a stack of rows; each row's icon disc is rendered
// on the card background so it "punches through" the line. Headings (day
// labels, week labels, run-status labels) sit outside the rail so they read
// as anchors instead of belonging to a row.
//
// Promoted in iter 26 from `features/transactions/activity-timeline.tsx`
// (open-coded since iter 5). The iter-5 drift note explicitly queued this
// primitive once a second timeline surface needed it; we ship it now even
// with a single consumer so future surfaces (rule run history, per-connection
// sync log, agent activity) inherit one vocabulary instead of forking.
//
// Composition: <TimelineRail> renders the wrapper spacing; nest
// <TimelineRail.Group> children with optional `label` (day heading); inside
// the group render any number of <TimelineRail.Row> children. Each row takes
// an `icon` (Lucide component) and arbitrary children for the content.

interface TimelineRailProps extends React.HTMLAttributes<HTMLDivElement> {
  // Vertical rhythm between groups. Defaults to `space-y-5` — matches the
  // ActivityTimeline original spacing. Override with `className` if a
  // denser/looser host (e.g. a sidebar) wants different.
  className?: string;
}

function TimelineRailRoot({ className, children, ...rest }: TimelineRailProps) {
  return (
    <div className={cn("space-y-5", className)} {...rest}>
      {children}
    </div>
  );
}

interface TimelineRailGroupProps extends React.HTMLAttributes<HTMLDivElement> {
  // Anchor heading for the group (e.g. "Today", "Yesterday", "Run #142").
  // Renders outside the rail so it reads as a section divider.
  label?: React.ReactNode;
  className?: string;
  listClassName?: string;
}

function TimelineRailGroup({
  label,
  className,
  listClassName,
  children,
  ...rest
}: TimelineRailGroupProps) {
  return (
    <div className={cn("space-y-3", className)} {...rest}>
      {label !== undefined && label !== null && (
        <Eyebrow as="h3">{label}</Eyebrow>
      )}
      {/* Subtle vertical rail behind the icons gives the feed a sense of
          continuity without leaning on dividers between rows. Icons sit on
          a card background to "punch through" the line. */}
      <ol
        className={cn(
          "border-border/60 relative space-y-3 border-l pl-0",
          listClassName,
        )}
      >
        {children}
      </ol>
    </div>
  );
}

interface TimelineRailRowProps extends React.HTMLAttributes<HTMLLIElement> {
  // Icon disc on the rail. Required — the primitive's identity is the
  // punched-through disc.
  icon: LucideIcon;
  // Optional muted/strikethrough rendering — for soft-deleted or otherwise
  // de-emphasised rows. Two intensities so consumers don't have to fork a
  // class string.
  muted?: boolean | "icon-only";
  className?: string;
  iconClassName?: string;
  // Optional class for the inner content column (everything to the right of
  // the disc). Defaults to `min-w-0 flex-1 py-0.5` matching ActivityTimeline.
  contentClassName?: string;
}

function TimelineRailRow({
  icon: Icon,
  muted = false,
  className,
  iconClassName,
  contentClassName,
  children,
  ...rest
}: TimelineRailRowProps) {
  const iconMuted = muted === true || muted === "icon-only";
  const contentMuted = muted === true;
  return (
    <li className={cn("relative flex gap-3 pl-3", className)} {...rest}>
      <div
        className={cn(
          // -ml + the rail-aligned icon disc. The negative margin equals the
          // pl-3 (0.75rem) + half the disc (0.875rem = size-7/2) + the 1px
          // rail, so the disc centres exactly on the rail.
          "bg-card border-border/60 text-muted-foreground -ml-[calc(0.875rem+1px)] flex size-7 shrink-0 items-center justify-center rounded-full border",
          iconMuted && "opacity-50",
          iconClassName,
        )}
      >
        <Icon className="size-3.5" />
      </div>
      <div
        className={cn(
          "min-w-0 flex-1 py-0.5",
          contentMuted && "opacity-60",
          contentClassName,
        )}
      >
        {children}
      </div>
    </li>
  );
}

// Compound export — `TimelineRail.Group` / `TimelineRail.Row` matches the
// shadcn-style composition used by Card, Table, Sidebar, etc.
export const TimelineRail = Object.assign(TimelineRailRoot, {
  Group: TimelineRailGroup,
  Row: TimelineRailRow,
});

export type {
  TimelineRailProps,
  TimelineRailGroupProps,
  TimelineRailRowProps,
};
