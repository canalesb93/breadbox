import * as React from "react";
import type { LucideIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

/**
 * Inline meta badge — the canonical "tiny status chip" used in list rows and
 * detail headers to label a row's secondary state (Hidden, Excluded, Linked,
 * Re-auth, System, Paused, …). Owns the v2 density vocabulary so the chip is
 * never hand-rolled:
 *
 *   text-[10px]  +  gap-1  +  px-1.5 py-0  +  [&>svg]:size-2.5
 *
 * Tone routes through the underlying `<Badge>` variant. The default tone is
 * `outline` because the chip is a *meta* label, not the row's primary
 * classification — a louder primary badge belongs on `<Badge>` directly.
 *
 * Use `muted` to opt into the `text-muted-foreground font-normal` shading that
 * the categories list uses for "System" / "Hidden" labels (so they don't
 * compete with the category name). For a tone-specific chip with custom
 * colours (e.g. the amber `Re-auth` pill in the accounts list) pass
 * `className` and use `variant="outline"` — the density tokens still apply,
 * which is the whole point.
 */
type MetaBadgeProps = React.ComponentProps<typeof Badge> & {
  icon?: LucideIcon;
  /** Apply the muted-foreground + font-normal shading used for low-emphasis labels. */
  muted?: boolean;
};

export function MetaBadge({
  icon: Icon,
  muted,
  className,
  children,
  variant = "outline",
  ...rest
}: MetaBadgeProps) {
  return (
    <Badge
      variant={variant}
      data-slot="meta-badge"
      className={cn(
        "gap-1 px-1.5 py-0 text-[10px] [&>svg]:size-2.5",
        muted && "text-muted-foreground font-normal",
        className,
      )}
      {...rest}
    >
      {Icon ? <Icon aria-hidden="true" /> : null}
      {children}
    </Badge>
  );
}
