import * as React from "react";
import { cn } from "@/lib/utils";

interface ColorRailCardProps extends React.HTMLAttributes<HTMLDivElement> {
  /**
   * CSS color value for the 4px left rail. Pass a CSS variable
   * (`"var(--destructive)"`) or a literal (`"#f97316"`). When omitted
   * the rail collapses to `--muted` so the card still reads as a hero,
   * but the colour stops carrying meaning.
   */
  accent?: string | null;
  /** Optional class applied to the bordered card wrapper. */
  cardClassName?: string;
  /**
   * Optional footer slot rendered with a top border + tinted muted
   * background — matches the inline action strip used on the
   * transaction- and account-detail heroes.
   */
  footer?: React.ReactNode;
  /** Optional class applied to the footer wrapper. */
  footerClassName?: string;
  children: React.ReactNode;
}

// ColorRailCard is the canonical "detail-page hero card" container — a
// bordered card with a 4px coloured left rail that encodes meaning
// (category color for transactions, accounting role for accounts,
// category's own color on the category-detail page). Sibling of
// `<SectionCard>` and `<ListCard>`; this is the third primitive in the
// v2 design vocabulary established by iter 5 (TX-detail hero) and iter 6
// (Account-detail hero, where the iter-5/6 drift note explicitly called
// for extraction once a third surface adopts it).
//
// Visual contract:
//   `bg-card relative overflow-hidden rounded-xl border`
//     `<div aria-hidden className="absolute inset-y-0 left-0 w-1" />`  ← rail
//     children
//     optional `<div className="border-t bg-muted/20 ...">footer</div>`
//
// The colour-rail principle: the rail's tint encodes *meaning* (asset vs
// liability, this transaction's category, this category's own colour),
// not decoration. Excluded / muted states collapse to `--muted` so the
// card reads "shelved" rather than "demands attention". Always pair the
// rail with a small uppercase eyebrow so colour alone never carries the
// signal.
export function ColorRailCard({
  accent,
  cardClassName,
  className,
  footer,
  footerClassName,
  children,
  ...rest
}: ColorRailCardProps) {
  return (
    <div
      className={cn(
        "bg-card relative overflow-hidden rounded-xl border",
        cardClassName,
        className,
      )}
      {...rest}
    >
      <div
        aria-hidden
        className="absolute inset-y-0 left-0 w-1"
        style={{ backgroundColor: accent ?? "var(--muted)" }}
      />
      {children}
      {footer && (
        <div
          className={cn(
            "border-t bg-muted/20 flex flex-wrap items-center justify-end gap-2 px-5 py-3 sm:px-7",
            footerClassName,
          )}
        >
          {footer}
        </div>
      )}
    </div>
  );
}
