import * as React from "react";
import { cn } from "@/lib/utils";

interface EyebrowProps extends React.HTMLAttributes<HTMLElement> {
  /**
   * Element to render. Defaults to `span` so the primitive composes inside
   * inline contexts; pass `"p"`, `"h3"`, etc. when the eyebrow is a block
   * label (section headers, hero eyebrows). Pass `"label"` (paired with
   * `htmlFor`) when the eyebrow doubles as a form-control label. Avoid
   * heading levels that conflict with the document outline — pick `"div"`
   * or `"span"` for non-semantic decoration.
   */
  as?: "span" | "p" | "div" | "h3" | "h4" | "label";
  /**
   * Forwarded to the underlying element when `as="label"`. Ignored
   * otherwise (React will warn if you pass it to a non-label element).
   */
  htmlFor?: string;
  /**
   * Visual emphasis. `default` is the standard `text-[10px]
   * tracking-[0.1em]` muted eyebrow used on detail-page section headers,
   * "Jump to" pills, and the timeline-rail day-heading. `hero` is the
   * slightly tighter-cap `tracking-[0.12em]` variant used in
   * detail-page hero columns ("Liability" / "Income" / "Category") where
   * the eyebrow sits directly under a large display title and benefits
   * from the extra letter air. `page` is the slightly heavier
   * `text-[11px] tracking-[0.08em]` rhythm used at the *page* scale —
   * `<PageHeader>` eyebrow, the home-stats KPI cell labels, and the
   * provider-card "Provider" caption — where the eyebrow has to hold its
   * own next to a 2xl–3xl title without disappearing. `nav` is the
   * heavier `font-semibold` cousin used by sidebar / shortcut-sheet
   * group labels (`text-[10px] tracking-[0.08em]`) where the eyebrow
   * sits inside chrome rather than next to content and needs the extra
   * weight to read against a coloured surface.
   */
  variant?: "default" | "hero" | "page" | "nav";
  className?: string;
  children: React.ReactNode;
}

// Eyebrow is the canonical uppercase micro-label used across detail pages,
// hero cards, timeline rails, and sidebar/menu group labels:
// `text-muted-foreground text-[10px] font-medium tracking-[0.1em]
// uppercase` is the default; `font-semibold` cousins live behind the
// `nav` variant. Don't reach for raw `text-[10px] font-medium/semibold
// tracking-* uppercase` markup — extend this primitive with a new
// variant if a host needs a different rhythm.
export function Eyebrow({
  as: Tag = "span",
  variant = "default",
  className,
  children,
  ...rest
}: EyebrowProps) {
  return (
    <Tag
      className={cn(
        "text-muted-foreground uppercase",
        variant === "nav" ? "font-semibold" : "font-medium",
        variant === "default" && "text-[10px] tracking-[0.1em]",
        variant === "hero" && "text-[10px] tracking-[0.12em]",
        variant === "page" && "text-[11px] tracking-[0.08em]",
        variant === "nav" && "text-[10px] tracking-[0.08em]",
        className,
      )}
      {...rest}
    >
      {children}
    </Tag>
  );
}
