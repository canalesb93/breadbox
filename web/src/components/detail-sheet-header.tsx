import * as React from "react";
import { SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Eyebrow } from "@/components/eyebrow";
import { cn } from "@/lib/utils";

interface DetailSheetHeaderProps {
  /**
   * Lucide icon component rendered inside the leading icon tile. The tile uses
   * the same `bg-muted text-muted-foreground rounded-lg border` vocabulary as
   * `StatusPanel`, `EmptyState`, and `SectionCard` so every v2 Sheet reads as
   * part of the system instead of a stock shadcn surface.
   */
  icon: React.ComponentType<{ className?: string }>;
  /** Optional uppercase eyebrow above the title (e.g. "New connection"). */
  eyebrow?: React.ReactNode;
  title: React.ReactNode;
  description?: React.ReactNode;
  /**
   * Visual density. `default` is the lighter shortcut-sheet rhythm
   * (`size-9` tile + `p-5` + base title); `accent` is the heavier
   * Connect-bank rhythm (`size-10` tile + `bg-muted/20 p-6` + `text-lg`
   * title) used by the primary flow Sheets. Both share the icon-tile lockup
   * + title/description stack.
   */
  density?: "default" | "accent";
  /** Optional trailing slot (e.g. status pill, badge). */
  trailing?: React.ReactNode;
  className?: string;
}

// DetailSheetHeader is the canonical icon-tile header for v2 Sheets. The
// shape — leading icon tile + optional eyebrow + title + description — was
// established by `ShortcutSheet` (iter 39) and adopted again by
// `ConnectBankSheet` (iter 40); promoted into a shared primitive in iter 41
// once two consumers shared it. Don't fork the lockup — pass `density` and
// `trailing` rather than re-opening `<SheetHeader>` inline.
//
// Visual contract:
//   <SheetHeader class="gap-3 border-b {p-5|bg-muted/20 p-6}">
//     row: icon-tile (size-9|10 rounded-lg bg-muted border)
//        + (eyebrow ↳ title ↳ description) column
//        + optional trailing slot
export function DetailSheetHeader({
  icon: Icon,
  eyebrow,
  title,
  description,
  density = "default",
  trailing,
  className,
}: DetailSheetHeaderProps) {
  const isAccent = density === "accent";
  return (
    <SheetHeader
      className={cn(
        "gap-3 border-b",
        isAccent
          ? "bg-muted/20 px-6 pb-6 pt-[calc(1.5rem+env(safe-area-inset-top))]"
          : "px-5 pb-5 pt-[calc(1.25rem+env(safe-area-inset-top))]",
        className,
      )}
    >
      <div className="flex items-start gap-3">
        <span
          aria-hidden
          className={cn(
            "bg-muted text-muted-foreground flex shrink-0 items-center justify-center rounded-lg border",
            isAccent ? "size-10" : "size-9",
          )}
        >
          <Icon className={isAccent ? "size-5" : "size-4"} />
        </span>
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          {eyebrow ? <Eyebrow as="p">{eyebrow}</Eyebrow> : null}
          <SheetTitle
            className={cn(
              "leading-tight",
              isAccent ? "text-lg font-semibold" : "text-base",
            )}
          >
            {title}
          </SheetTitle>
          {description ? (
            <SheetDescription
              className={cn(isAccent ? "text-sm" : "mt-0.5 text-xs")}
            >
              {description}
            </SheetDescription>
          ) : null}
        </div>
        {trailing ? (
          <div className="ms-2 flex shrink-0 items-start">{trailing}</div>
        ) : null}
      </div>
    </SheetHeader>
  );
}
