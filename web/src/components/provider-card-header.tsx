import * as React from "react";
import { cn } from "@/lib/utils";
import { Eyebrow } from "@/components/eyebrow";

interface ProviderCardHeaderProps {
  /**
   * Icon node, typically a `lucide-react` icon at `size-5`. Rendered inside
   * the rounded tone tile on the left edge of the header.
   */
  icon: React.ReactNode;
  /**
   * Tailwind classes for the icon tile background/text color. Examples:
   * `"bg-blue-500/10 text-blue-600 dark:text-blue-400"` (Plaid),
   * `"bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"` (Teller),
   * `"bg-amber-500/10 text-amber-600 dark:text-amber-400"` (CSV).
   */
  iconClassName: string;
  /** Provider display name, e.g. "Plaid", "Teller", "CSV import". */
  title: string;
  /** One-line marketing description. Capped at `max-w-md`. */
  description: string;
  /**
   * Optional inline badge rendered to the right of the title — typically a
   * `<ProviderStatusBadge>`. CSV omits this because there are no credentials
   * to gate.
   */
  badge?: React.ReactNode;
  /**
   * Right-aligned slot, typically a `<ProviderScoreboard>`. Docks below
   * the identity column on mobile and to the right on ≥640px.
   */
  trailing?: React.ReactNode;
}

// ProviderCardHeader is the canonical header body inside a provider settings
// card — the layer that sits one level inside `<ColorRailCard>` on the Plaid,
// Teller, and CSV provider pages. Promoted in iter 98 from three near-byte-
// identical header blocks (each ~25 LOC). 26th shared primitive in the v2
// vocabulary.
//
// Visual contract:
//   Wrapper: `flex flex-col gap-5 px-6 py-5
//             sm:flex-row sm:items-center sm:justify-between sm:px-7`
//   Identity column: icon tile (size-11 rounded-lg) + eyebrow "Provider" +
//     title row (h2 + optional badge) + description.
//   Trailing column: scoreboard slot, right-docked on ≥640px.
//
// The breakpoints encode the same density story for every provider card:
// mobile stacks the scoreboard under the identity column; ≥640px aligns
// them on a single baseline. Always render inside `<ColorRailCard>` —
// ProviderCardHeader is the body layer, not the chrome.
export function ProviderCardHeader({
  icon,
  iconClassName,
  title,
  description,
  badge,
  trailing,
}: ProviderCardHeaderProps) {
  return (
    <div className="flex flex-col gap-5 px-6 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-7">
      <div className="flex min-w-0 items-start gap-3">
        <div
          className={cn(
            "flex size-11 shrink-0 items-center justify-center rounded-lg",
            iconClassName,
          )}
        >
          {icon}
        </div>
        <div className="min-w-0 space-y-1">
          <Eyebrow as="p" variant="page">
            Provider
          </Eyebrow>
          {badge ? (
            <div className="flex items-center gap-2">
              <h2 className="text-foreground text-lg font-semibold tracking-tight">
                {title}
              </h2>
              {badge}
            </div>
          ) : (
            <h2 className="text-foreground text-lg font-semibold tracking-tight">
              {title}
            </h2>
          )}
          <p className="text-muted-foreground max-w-md text-sm">{description}</p>
        </div>
      </div>
      {trailing}
    </div>
  );
}
