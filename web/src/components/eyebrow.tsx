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
   * from the extra letter air.
   */
  variant?: "default" | "hero";
  className?: string;
  children: React.ReactNode;
}

// Eyebrow is the canonical uppercase micro-label used across detail pages,
// hero cards, and timeline rails: `text-muted-foreground text-[10px]
// font-medium tracking-[0.1em] uppercase`. The pattern was open-coded
// across ten files in five subtly different sizes/trackings before iter 37
// consolidated them. Don't reach for raw `text-[10px] font-medium
// tracking-* uppercase` markup again — extend this primitive with a new
// variant if a hero needs a different rhythm.
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
        "text-muted-foreground font-medium uppercase",
        variant === "default" && "text-[10px] tracking-[0.1em]",
        variant === "hero" && "text-[10px] tracking-[0.12em]",
        className,
      )}
      {...rest}
    >
      {children}
    </Tag>
  );
}
