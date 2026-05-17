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

// StatusPanel renders the canonical inline tone-tinted status block:
// tinted icon tile + heading + body on a muted surface. Tone is carried
// by the icon tile alone — the earlier 3px tone-tinted left rail was
// removed once the icon proved to be the dominant signal and the rail
// added visual weight without adding scannability.
//
// Originally established inline in `routes/setup-account.tsx` (iter 14) for
// already-setup + invalid-token states. Promoted in iter 16 once the
// Providers page needed the same vocabulary for env-locked + ENCRYPTION_KEY
// warnings. Don't fork the look — change this primitive.
//
// Visual contract:
//   `bg-muted/30 overflow-hidden rounded-md border p-4`
//     row: tinted icon tile (size-8) + heading + body
//     optional trailing slot at the right edge
const PALETTES: Record<StatusPanelTone, { iconBg: string }> = {
  success: {
    iconBg: "bg-success/12 text-success",
  },
  destructive: {
    iconBg: "bg-destructive/10 text-destructive",
  },
  warning: {
    iconBg: "bg-amber-500/10 text-amber-600 dark:text-amber-400",
  },
  info: {
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
        "bg-muted/30 overflow-hidden rounded-md border p-4",
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
