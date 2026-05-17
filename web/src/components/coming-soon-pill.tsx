import * as React from "react";
import { Clock, type LucideIcon } from "lucide-react";

import { cn } from "@/lib/utils";

/**
 * ComingSoonPill — the canonical muted "Coming soon" status pill rendered
 * in the trailing slot of a `<StatusPanel tone="info">` for unbuilt
 * surfaces.
 *
 * Vocabulary: muted background + muted-foreground label, fully-rounded
 * pill (`rounded-full`), `px-2.5 py-1 text-[11px]`, uppercase + wide
 * tracking, leading `Clock` icon at `size-3`. Distinct from the iter-37
 * `<Eyebrow>` (no chip, used as a section/page micro-label) and from
 * shadcn `<Badge>` (rectangular, semantic-tone variants) — this is a
 * neutral inline status chip whose only job is to caption "not yet."
 *
 * Two consumers share it today: `routes/placeholder.tsx` (the canonical
 * unbuilt-nav-leaf shell, iter 21) and `components/settings-shell.tsx`
 * (the in-the-works settings section panel, iter 78). Both previously
 * hand-rolled a 10-class span — promoted in iter 102 so the pill shape,
 * padding, and rhythm live in one place. Sibling of `<JumpToPill>` in
 * the iter-30 "primary-tinted pill triad" drift note: when a new
 * surface needs the muted variant, reach for this; when it needs the
 * primary-tinted v2-chip variant, that one is still inlined in
 * `BrandHeader` / `AuthShell` until the third surface adopts it.
 *
 * `icon` defaults to `Clock` (matches both live consumers); pass a
 * different `LucideIcon` if a new caller wants a different leading
 * glyph without losing the chip rhythm. `children` is the label —
 * keep it short ("Coming soon", "In review", "Beta"). If a non-muted
 * tone is ever needed, add a `tone` prop here rather than forking the
 * className — keep the vocabulary in one file.
 */
export interface ComingSoonPillProps
  extends Omit<React.HTMLAttributes<HTMLSpanElement>, "children"> {
  /** Leading lucide icon; defaults to `Clock`. */
  icon?: LucideIcon;
  /** The pill label — defaults to "Coming soon". */
  children?: React.ReactNode;
}

export function ComingSoonPill({
  icon: Icon = Clock,
  className,
  children = "Coming soon",
  ...rest
}: ComingSoonPillProps) {
  return (
    <span
      className={cn(
        "bg-muted text-muted-foreground inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-medium tracking-wide uppercase",
        className,
      )}
      {...rest}
    >
      <Icon className="size-3" />
      {children}
    </span>
  );
}
