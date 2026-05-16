import * as React from "react";
import { cn } from "@/lib/utils";

export type StatusPanelTone = "success" | "destructive" | "warning" | "info";

interface StatusPanelProps {
  tone: StatusPanelTone;
  icon: React.ComponentType<{ className?: string }>;
  heading: React.ReactNode;
  body?: React.ReactNode;
  /** Optional trailing slot (e.g. small action button or pill). */
  trailing?: React.ReactNode;
  className?: string;
}

// StatusPanel renders the canonical inline tone-tinted status block: 3px
// left rail in the tone colour + tinted icon tile + heading + body. Same
// "colour encodes meaning" principle as ColorRailCard, but inline-only and
// smaller — meant for "this surface is in state X" notices that aren't the
// primary hero.
//
// Originally established inline in `routes/setup-account.tsx` (iter 14) for
// already-setup + invalid-token states. Promoted in iter 16 once the
// Providers page needed the same vocabulary for env-locked + ENCRYPTION_KEY
// warnings. Don't fork the look — change this primitive.
//
// Visual contract:
//   `bg-muted/30 relative overflow-hidden rounded-md border p-4 pl-5`
//     `before:` 3px tone-tinted left rail
//     row: tinted icon tile (size-8) + heading + body
//     optional trailing slot at the right edge
const PALETTES: Record<
  StatusPanelTone,
  { rail: string; iconBg: string }
> = {
  success: {
    rail: "before:bg-success",
    iconBg: "bg-success/12 text-success",
  },
  destructive: {
    rail: "before:bg-destructive",
    iconBg: "bg-destructive/10 text-destructive",
  },
  warning: {
    rail: "before:bg-amber-500",
    iconBg: "bg-amber-500/10 text-amber-600 dark:text-amber-400",
  },
  info: {
    rail: "before:bg-muted-foreground/40",
    iconBg: "bg-muted text-muted-foreground",
  },
};

export function StatusPanel({
  tone,
  icon: Icon,
  heading,
  body,
  trailing,
  className,
}: StatusPanelProps) {
  const palette = PALETTES[tone];
  return (
    <div
      className={cn(
        "bg-muted/30 relative overflow-hidden rounded-md border p-4 pl-5 before:absolute before:top-0 before:bottom-0 before:left-0 before:w-[3px]",
        palette.rail,
        className,
      )}
    >
      <div className="flex items-start gap-3">
        <span
          className={cn(
            "flex size-8 shrink-0 items-center justify-center rounded-md",
            palette.iconBg,
          )}
        >
          <Icon className="size-4" />
        </span>
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="text-foreground text-sm font-medium">{heading}</span>
          {body && (
            <span className="text-muted-foreground text-xs leading-relaxed">
              {body}
            </span>
          )}
        </div>
        {trailing && (
          <div className="ml-2 flex shrink-0 items-center">{trailing}</div>
        )}
      </div>
    </div>
  );
}
