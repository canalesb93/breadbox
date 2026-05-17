import * as React from "react";
import { cn } from "@/lib/utils";

type FormFooterInset = "card" | "sheet";

interface FormFooterProps {
  /**
   * Primary action — submit button (right). Optional so the footer can host
   * a single Cancel-only state (e.g. the CSV drop stage, where the user
   * hasn't picked a file yet).
   */
  primary?: React.ReactNode;
  /** Secondary action — typically Cancel (left of primary). */
  secondary?: React.ReactNode;
  /** Optional helper text rendered left-aligned (e.g. validation hint). */
  hint?: React.ReactNode;
  /**
   * Which container is the strip flushing to?
   * - `"card"` (default) — inside a `<SectionCard>` body (`px-5 py-5`);
   *   the strip negates to `-mx-5 -mb-5` so the muted footer lines up with
   *   the card border.
   * - `"sheet"` — inside a `<Sheet>` body (`p-6`); the strip negates to
   *   `-mx-6 -mb-6` and uses `mt-auto` so it sticks to the Sheet bottom
   *   when the form body is short.
   */
  inset?: FormFooterInset;
  className?: string;
}

// FormFooter renders the canonical flush bordered action strip at the bottom
// of a form container. Established in iter 13 on api-key-new
// (`features/api-keys/api-key-form.tsx`); promoted in iter 15 once
// tag-form + category-form adopted it.
//
// Visual contract:
//   `<div className="bg-muted/20 flex items-center justify-end gap-2
//                    border-t px-{INSET} py-3 -mx-{INSET} -mb-{INSET}">`
//
// The negative margins push the strip out to the parent container's edges
// so the top border lines up with the outer border and the action strip
// reads as a footer of the container, not as floating content. Iter 49
// folded the iter-48 `CsvFooterStrip` into here as the `inset="sheet"`
// variant — same visual vocabulary, different padding scale.
//
// Don't fork the look — extend this primitive (add an inset variant) when a
// new form container shape needs a flush footer.
export function FormFooter({
  primary,
  secondary,
  hint,
  inset = "card",
  className,
}: FormFooterProps) {
  return (
    <div
      className={cn(
        "bg-muted/20 flex flex-wrap items-center justify-end gap-2 border-t",
        inset === "card"
          ? "-mx-5 -mb-5 px-5 py-3"
          : "-mx-6 -mb-6 mt-auto px-6 py-3",
        hint && "sm:justify-between",
        className,
      )}
    >
      {hint && (
        <div className="text-muted-foreground order-last min-w-0 text-xs sm:order-first">
          {hint}
        </div>
      )}
      <div className="ml-auto flex items-center gap-2">
        {secondary}
        {primary}
      </div>
    </div>
  );
}
