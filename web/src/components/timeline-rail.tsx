import * as React from "react";
import { type LucideIcon } from "lucide-react";
import { Eyebrow } from "@/components/eyebrow";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

// TimelineRail is the v2 vocabulary for a vertical activity feed: a thin
// vertical rail anchors a stack of rows; each row's icon disc is rendered on
// the card background so it "punches through" the line. Headings (day
// labels, week labels, run-status labels) sit outside the rail as anchors.
//
// The rail is drawn per-row via an `::before` pseudo-element on each `<li>`
// and clipped to the disc centre on the first and last row of each group via
// `:first-of-type` / `:last-of-type`. Middle rows draw a full-height segment
// so the line reads as continuous within each group. (A previous `<ol
// border-l>` implementation made the line extend past the last disc.)
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

// Semantic tone for the icon disc. Encodes the *kind* of event so a
// scanning eye can pick out rule applications vs sync events vs comments
// without parsing the summary line. Default `neutral` matches the legacy
// look — switch to a tinted variant when the event carries a clear
// semantic role (sync, rule fire, classification change, etc.). Tones
// adapt to light/dark themes via the standard shadcn vocabulary
// (primary / emerald / amber / sky / destructive) used elsewhere in v2
// (StatusPanel, ColorRailCard, MetaBadge). Iter 93; `destructive` added
// in iter 105 to give sync-history rows a "this run failed" disc accent
// that matches StatusPanel's destructive vocabulary — `warning` (amber)
// is the wrong colour for an *errored* sync run; it belongs to "tag
// removed" / "soft warning" semantics.
type TimelineRailTone =
  | "neutral"
  | "primary"
  | "success"
  | "warning"
  | "destructive"
  | "info"
  | "muted";

// TONE_CLASSES tints both the disc *border* and the inner icon so the
// accent reads on both — a bare-coloured icon on a neutral border looks
// like a colour smudge, and a coloured border with a neutral icon looks
// like the disc itself is wrong. Backgrounds stay `bg-card` (set on the
// disc base in TimelineRailRow) so the disc still punches through the
// rail line.
const TONE_CLASSES: Record<TimelineRailTone, string> = {
  neutral: "border-border/60 text-muted-foreground",
  primary: "border-primary/40 text-primary",
  success: "border-emerald-500/40 text-emerald-600 dark:text-emerald-400",
  warning: "border-amber-500/40 text-amber-600 dark:text-amber-400",
  // `destructive` matches StatusPanel / ColorRailCard's destructive tone
  // (failed-state vocabulary). Use for hard failure rows (errored sync
  // runs, failed rule runs) — distinct from `warning` (soft / removed).
  destructive: "border-destructive/40 text-destructive",
  info: "border-sky-500/40 text-sky-600 dark:text-sky-400",
  muted: "border-border/60 text-muted-foreground/70",
};

interface TimelineRailRowProps extends React.HTMLAttributes<HTMLLIElement> {
  // Icon disc on the rail. Required — the primitive's identity is the
  // punched-through disc.
  icon: LucideIcon;
  // Semantic tone for the icon disc — see `TimelineRailTone`. Defaults
  // to `neutral` (the legacy look). Combines with `muted` — a
  // `tone="primary" muted` row keeps the primary tint at reduced opacity.
  tone?: TimelineRailTone;
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
  tone = "neutral",
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
        // `pl-3.5` (14px) plus the disc's `-ml-3.5` (-14px) sits the
        // disc's left edge at x=0 and its centre at x=14px (half of the
        // 28px-wide disc) measured from the row's outer-left edge.
        "relative flex gap-3 pl-3.5",
        // Rail line drawn via `::before`. 1px wide, centred on the disc
        // centre at x=14px → left = 14px - 0.5px = calc(0.875rem - 0.5px).
        "before:content-[''] before:absolute before:left-[calc(0.875rem-0.5px)] before:w-px before:bg-border/60",
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
          // pseudo behind it. Tone classes layered after the base set the
          // border + icon tint per event kind.
          "bg-card relative z-10 -ml-3.5 flex size-7 shrink-0 items-center justify-center rounded-full border",
          TONE_CLASSES[tone],
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

interface TimelineRailRowSkeletonProps
  extends Omit<React.HTMLAttributes<HTMLLIElement>, "children"> {
  // Whether the row carries a body block under the headline (e.g. a comment
  // bubble). Defaults to false so the skeleton matches the dominant row shape
  // (single-line summary + timestamp).
  body?: boolean;
  className?: string;
}

// RowSkeleton mirrors the real <TimelineRail.Row> geometry — same disc + rail
// + content column — so loading-to-loaded transitions don't jump. Reach for
// this inside a <TimelineRail.Group> with the same `label` you'll render for
// the real rows (or no label for a flat feed). Added in iter 65; previously
// the activity timeline hand-rolled a `gap-3` skeleton with a `size-7
// rounded-full` chip that didn't carry the rail line, so the loading state
// shifted layout when annotations arrived.
function TimelineRailRowSkeleton({
  body = false,
  className,
  ...rest
}: TimelineRailRowSkeletonProps) {
  return (
    <li
      className={cn(
        // Geometry mirrors TimelineRailRow exactly so the disc lands on the
        // rail x-axis and the rail pseudo-element clips identically — see the
        // long-form comment on TimelineRailRow for the math.
        "relative flex gap-3 pl-3.5",
        "before:content-[''] before:absolute before:left-[calc(0.875rem-0.5px)] before:w-px before:bg-border/60",
        "before:-top-3 before:-bottom-3",
        "[&:first-of-type]:before:top-[14px]",
        "[&:last-of-type]:before:bottom-[calc(100%-14px)]",
        className,
      )}
      {...rest}
    >
      <Skeleton
        aria-hidden
        className="bg-card border-border/60 relative z-10 -ml-3.5 size-7 shrink-0 rounded-full border"
      />
      <div className="min-w-0 flex-1 space-y-1.5 py-1">
        <Skeleton className="h-3 w-3/4" />
        {body && <Skeleton className="h-8 w-full rounded-md" />}
        <Skeleton className="h-3 w-1/4" />
      </div>
    </li>
  );
}

// Compound export — `TimelineRail.Group` / `TimelineRail.Row` matches the
// shadcn-style composition used by Card, Table, Sidebar, etc.
export const TimelineRail = Object.assign(TimelineRailRoot, {
  Group: TimelineRailGroup,
  Row: TimelineRailRow,
  RowSkeleton: TimelineRailRowSkeleton,
});

export type { TimelineRailTone };
