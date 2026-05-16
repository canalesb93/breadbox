import * as React from "react";
import { cn } from "@/lib/utils";

interface FormFooterProps {
  /** Primary action — submit button (right). */
  primary: React.ReactNode;
  /** Secondary action — typically Cancel (left of primary). */
  secondary?: React.ReactNode;
  /** Optional helper text rendered left-aligned (e.g. validation hint). */
  hint?: React.ReactNode;
  className?: string;
}

// FormFooter renders the canonical flush bordered action strip at the bottom
// of a `<SectionCard>` that wraps a form. Established in iter 13 on
// api-key-new (`features/api-keys/api-key-form.tsx`); the iter-13 drift
// note explicitly queued promotion to a primitive once a second/third form
// adopts it. Iter 15 brings tag-form and category-form onto the pattern, so
// it lives here.
//
// Visual contract (matches the api-key-new pattern):
//   `<div className="bg-muted/20 -mx-5 -mb-5 mt-2 flex items-center justify-end
//                    gap-2 border-t px-5 py-3">`
//
// The negative margins (`-mx-5 -mb-5`) push the strip out to the SectionCard
// body's edges so the top border lines up with the card's outer border and
// the action strip reads as a footer of the card, not as floating content.
// Drop it inside a SectionCard with the default `bodyClassName="px-5 py-5"`.
//
// Don't fork the look — change this primitive instead.
export function FormFooter({
  primary,
  secondary,
  hint,
  className,
}: FormFooterProps) {
  return (
    <div
      className={cn(
        "bg-muted/20 -mx-5 -mb-5 mt-2 flex flex-wrap items-center justify-end gap-2 border-t px-5 py-3",
        hint && "sm:justify-between",
        className,
      )}
    >
      {hint && (
        <div className="text-muted-foreground order-last text-xs sm:order-first">
          {hint}
        </div>
      )}
      <div className="flex items-center gap-2">
        {secondary}
        {primary}
      </div>
    </div>
  );
}
