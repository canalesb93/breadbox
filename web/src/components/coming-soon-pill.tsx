import * as React from "react";
import { Clock, type LucideIcon } from "lucide-react";

import { cn } from "@/lib/utils";

/**
 * ComingSoonPill — the canonical muted "Coming soon" status pill, used in
 * the trailing slot of `<StatusPanel tone="info">` for unbuilt surfaces.
 *
 * Neutral inline status chip whose only job is to caption "not yet." Don't
 * fork the look — pass `icon` / `children` to vary the leading glyph or
 * label; if a non-muted tone is ever needed, add a `tone` prop here rather
 * than forking the className.
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
