import * as React from "react";
import { type LucideIcon } from "lucide-react";
import { Eyebrow } from "@/components/eyebrow";
import { cn } from "@/lib/utils";

// TimelineRail is the v2 vocabulary for a vertical activity feed: a thin
// vertical rail anchors a stack of rows; each row's icon disc is rendered on
// the card background so it "punches through" the line. Headings (day
// labels, week labels, run-status labels) sit outside the rail as anchors.
//
// Promoted in iter 26 from `features/transactions/activity-timeline.tsx`
// (open-coded since iter 5). Reworked in iter 56 to fix the rail-tail bug
// flagged by iter 55's audit: the per-group rail used `<ol border-l>`,
// which made the line visibly extend past the last row's icon disc — the
// disc was inset with negative margin so the `<ol>`'s left border kept
// going past the centred icon. We now draw the rail per-row via an
// `::before` pseudo-element on each `<li>` and clip it to the disc centre
// on the first and last row of each group via `:first-of-type` /
// `:last-of-type`. Middle rows draw a full-height segment so the line
// reads as continuous within each group.
//
// Composition (unchanged consumer API): <TimelineRail> renders the wrapper
// spacing; nest <TimelineRail.Group> children with optional `label` (day
// heading); inside the group render any number of <TimelineRail.Row>
// children. Each row takes an `icon` (Lucide component) and arbitrary
// children for the content.

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
        // Day-heading separator: a small dot anchored on the rail's x-axis
        // (matches the disc centre below), followed by the eyebrow label,
        // and a hairline rule that fills the remaining width. The dot +
        // hairline make the heading read as a *temporal divider inside the
        // timeline* instead of a generic section header — distinguishing it
        // from the SectionCard/CardHeader "Activity" title that sits above
        // the whole feed. Iter 62.
        //
        // Geometry: `pl-3.5` mirrors the rows below so the eyebrow's
        // baseline aligns vertically with row content. The 6px dot
        // (`size-1.5`) needs `-ml-[17px]` (= -14px row-padding − 3px
        // half-dot-width) so its centre sits exactly on the rail's x-axis
        // at x=0, matching the disc centres of the rows.
        <div className="flex items-center gap-2 pl-3.5">
          <span
            aria-hidden
            className="bg-border/80 size-1.5 shrink-0 rounded-full"
            style={{ marginLeft: "-17px" }}
          />
          <Eyebrow as="h3">{label}</Eyebrow>
          <span aria-hidden className="bg-border/60 h-px flex-1" />
        </div>
      )}
      {/* Rail is drawn per-row via `::before` on each <li>, not as a
          continuous border on the <ol>. This lets us clip the line to the
          first and last disc centres so the rail starts and ends exactly
          where the content does — no stray tail under the last row. */}
      <ol className={cn("relative space-y-3", listClassName)}>{children}</ol>
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
    <li
      className={cn(
        // `relative` so the rail `::before` positions against the row;
        // `pl-3.5` (14px) plus the disc's `-ml-3.5` lines the disc centre
        // up with the rail x-position at the row's left edge (x=0).
        "relative flex gap-3 pl-3.5",
        // Rail line drawn via `::before`. 1px wide at the disc x-centre
        // (left:0 of the row's padding-box, since the disc's negative
        // margin pulls it back so its centre sits on x=0). The pseudo
        // gets the same `border-border/60` token as the previous border.
        "before:content-[''] before:absolute before:left-0 before:w-px before:bg-border/60",
        // Middle rows: full height; the row's `space-y-3` (12px) gap with
        // the next sibling is bridged by `bottom: -12px` so the rail
        // appears unbroken between rows. `-top: 12px` mirrors that on the
        // top edge for parity (the first-of-type override below cancels it
        // on the first row).
        "before:-top-3 before:-bottom-3",
        // Clip the rail on the first row so it begins at the disc centre
        // (half of size-7 = 14px) rather than extending above into the
        // group's day heading.
        "[&:first-of-type]:before:top-[14px]",
        // Clip the rail on the last row so it ends at the disc centre
        // rather than extending below the group. `bottom: calc(100% -
        // 14px)` measured from the row's padding-box bottom edge lands
        // the line exactly on the disc centre.
        "[&:last-of-type]:before:bottom-[calc(100%-14px)]",
        className,
      )}
      {...rest}
    >
      <div
        className={cn(
          // Disc centred on the rail (x=0). `-ml-3.5` (-14px) pulls the
          // 28px-wide disc back so its centre sits on the row's left edge.
          // `relative z-10` + `bg-card` punches the disc through the rail
          // pseudo behind it.
          "bg-card border-border/60 text-muted-foreground relative z-10 -ml-3.5 flex size-7 shrink-0 items-center justify-center rounded-full border",
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
