import * as React from "react";
import {
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Eyebrow } from "@/components/eyebrow";
import { cn } from "@/lib/utils";

interface DetailDialogHeaderProps {
  /**
   * Lucide icon component rendered inside the leading icon tile. The tile uses
   * the same `bg-muted text-muted-foreground rounded-lg border` vocabulary as
   * `DetailSheetHeader`, `StatusPanel`, `EmptyState`, and `SectionCard` so
   * every v2 form-bearing Dialog reads as part of the system instead of a
   * stock shadcn surface.
   */
  icon: React.ComponentType<{ className?: string }>;
  /** Optional uppercase eyebrow above the title (e.g. "Household"). */
  eyebrow?: React.ReactNode;
  title: React.ReactNode;
  description?: React.ReactNode;
  /** Optional trailing slot (e.g. a small status pill). */
  trailing?: React.ReactNode;
  className?: string;
}

// DetailDialogHeader is the canonical icon-tile header for v2 centered
// Dialogs that host a form or a multi-step payload (Add member, Create
// login, Share setup link, etc.). Sibling of `<DetailSheetHeader>` (which
// owns the icon-tile header for slide-in Sheets) — same lockup, same icon
// tile vocabulary, just routed through `<DialogTitle>` / `<DialogDescription>`
// so it composes with shadcn `<Dialog>` rather than `<Sheet>`.
//
// Promoted in the iter 100 sweep that retired the destructive `<Dialog>`
// holdouts onto `<ConfirmDialog>` (which is for yes/no confirmations). The
// remaining centered `<Dialog>` consumers are the form-bearing modals
// (household-section's AddMember / CreateLogin / ShareLink) — they don't
// belong on `<ConfirmDialog>`, so this primitive gives them the same
// visual lift as the iter 41 `<DetailSheetHeader>` family. 27th shared
// primitive in the v2 vocabulary.
//
// Visual contract:
//   <DialogHeader class="gap-3 sm:text-left">
//     row: icon-tile (size-9 rounded-lg bg-muted border)
//        + (eyebrow ↳ title ↳ description) column
//        + optional trailing slot
//
// Don't fork the lockup — pass `trailing` / `eyebrow` rather than
// re-opening `<DialogHeader>` inline. If a new surface needs a stronger
// accent density, add a `density` prop here (mirroring the iter 41
// `<DetailSheetHeader>` `accent` shape) instead of bespoke chrome.
export function DetailDialogHeader({
  icon: Icon,
  eyebrow,
  title,
  description,
  trailing,
  className,
}: DetailDialogHeaderProps) {
  return (
    <DialogHeader className={cn("gap-3 sm:text-left", className)}>
      <div className="flex items-start gap-3">
        <span
          aria-hidden
          className="bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg border"
        >
          <Icon className="size-4" />
        </span>
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          {eyebrow ? <Eyebrow as="p">{eyebrow}</Eyebrow> : null}
          <DialogTitle className="text-base leading-tight">{title}</DialogTitle>
          {description ? (
            <DialogDescription className="mt-0.5 text-xs">
              {description}
            </DialogDescription>
          ) : null}
        </div>
        {trailing ? (
          <div className="ms-2 flex shrink-0 items-start">{trailing}</div>
        ) : null}
      </div>
    </DialogHeader>
  );
}
