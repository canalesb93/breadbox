import * as React from "react";

import { Button } from "@/components/ui/button";
import { Eyebrow } from "@/components/eyebrow";
import { cn } from "@/lib/utils";

/**
 * JumpToPill — the canonical detail-page "Jump to" pill button.
 *
 * Ghost-outline pill (`variant="outline"`, `size="sm"`, `h-7 px-2.5`),
 * 11px label text, `size-3` leading icon. Reads as a labelled lateral
 * link from a detail-page hero. Don't fork the look — pass props.
 *
 * `asChild` forwards to shadcn `Button.asChild` so consumers can wrap
 * a `<Link>` (the most common case) without losing keyboard ergonomics.
 */
export interface JumpToPillProps
  extends React.ComponentProps<typeof Button> {
  asChild?: boolean;
}

export function JumpToPill({
  className,
  asChild,
  children,
  ...rest
}: JumpToPillProps) {
  return (
    <Button
      variant="outline"
      size="sm"
      asChild={asChild}
      className={cn("h-7 gap-1.5 px-2.5 text-xs", className)}
      {...rest}
    >
      {children}
    </Button>
  );
}

/**
 * JumpToRow — the labelled "Jump to" eyebrow + pill cluster used in
 * detail-page hero columns. Children should be `<JumpToPill>` elements.
 */
export function JumpToRow({
  label = "Jump to",
  className,
  children,
}: {
  label?: React.ReactNode;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div
      className={cn("flex flex-wrap items-center gap-1.5", className)}
    >
      <Eyebrow className="mr-1">{label}</Eyebrow>
      {children}
    </div>
  );
}
