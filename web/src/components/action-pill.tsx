import * as React from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/**
 * ActionPill — the canonical small action button used inside
 * `<ColorRailCard footer>` strips and `<StatusPanel trailing>` slots.
 *
 * Vocabulary: 28px-tall pill (`h-7`), `text-xs` label, `gap-1.5`
 * between leading icon and label, `size-3.5` leading icon. Distinct
 * from `<JumpToPill>` (same `h-7` height, but `variant="outline"` +
 * `px-2.5` + `size-3` icon for lateral nav) and from a standalone CTA
 * (`<Button size="sm">` is 32px tall).
 *
 * The button has a real handler (`onClick`, or wraps a `<Link>` via
 * `asChild`) — it's a dispatched action, not a navigation pill. Tone
 * is governed by `variant`: `ghost` for action strips inside a card
 * surface (account-detail / connection-detail `<ColorRailCard footer>`),
 * `outline` for top-of-page `<StatusPanel trailing>` CTAs where the
 * pill needs slightly more visual weight.
 *
 * Eight surfaces share the recipe today; promoting it to a primitive
 * keeps the icon-size and padding constants in one place so future
 * tweaks (height, gap, icon size) propagate everywhere.
 *
 * `asChild` forwards to shadcn `Button.asChild` so consumers can wrap
 * a router `<Link>` without losing keyboard ergonomics.
 */
export interface ActionPillProps
  extends React.ComponentProps<typeof Button> {
  asChild?: boolean;
}

export function ActionPill({
  className,
  asChild,
  variant = "ghost",
  children,
  ...rest
}: ActionPillProps) {
  return (
    <Button
      variant={variant}
      size="sm"
      asChild={asChild}
      className={cn("h-7 gap-1.5 text-xs", className)}
      {...rest}
    >
      {children}
    </Button>
  );
}
