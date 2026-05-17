import { Link, type LinkProps } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/**
 * ViewAllPill — the canonical "card-header / footer goto" pill used when a
 * bordered surface (ListCard, SectionCard, ColorRailCard footer) wants to
 * defer to a fuller list page. Replaces the four open-coded variants:
 * home-recent-transactions ("View all" header pill), home-connections-panel
 * ("Manage" header pill), account-recent-transactions ("See all
 * transactions" footer button), and connection-detail sync-history ("View
 * all →" plain text link).
 *
 * Visual contract: ghost variant + `h-7` + `px-2` + `text-xs` +
 * `text-muted-foreground` → hover `text-foreground`, with a trailing
 * `<ArrowRight className="size-3" />` icon (matches the iter-75
 * `<JumpToPill>` size-3 leading-icon vocabulary so the two lateral-link
 * primitives speak the same icon language).
 *
 * The `-mr-2` shoulder lets it sit flush with the card's right padding when
 * used in a `<ListCard action>` / `<SectionCard action>` slot. Set
 * `align="footer"` to drop the shoulder (footer slots already supply their
 * own padding, so no margin shift is needed there).
 *
 * Distinct from:
 *   - `<JumpToPill>` (outline variant, lateral nav between detail-page heroes)
 *   - `<ActionPill>` (real-handler action, ghost/outline inside footer strips
 *     of `<ColorRailCard>` / `<StatusPanel trailing>`, `size-3.5` leading icon)
 *   - `<Button size="sm">` (regular 32px CTA)
 *
 * Don't open-code the recipe — pass props.
 */
export interface ViewAllPillProps {
  // Router link target. Forwarded to a TanStack `<Link>`.
  to: LinkProps["to"];
  // Optional search params for the link target. Forwarded as-is.
  search?: LinkProps["search"];
  // Optional path params for parameterised routes.
  params?: LinkProps["params"];
  // Visible label. Sentence-cased, short verb-led phrase ("View all" /
  // "Manage" / "See all transactions"). The trailing arrow comes from the
  // primitive — don't include it in the children.
  children: React.ReactNode;
  // Position context. `"header"` (default) applies the `-mr-2` flush
  // shoulder so the pill sits hard against the card's right edge inside a
  // `<ListCard action>` / `<SectionCard action>` slot. `"footer"` drops the
  // shoulder for use inside a `footer` slot.
  align?: "header" | "footer";
  className?: string;
}

export function ViewAllPill({
  to,
  search,
  params,
  children,
  align = "header",
  className,
}: ViewAllPillProps) {
  return (
    <Button
      asChild
      variant="ghost"
      size="sm"
      className={cn(
        "text-muted-foreground hover:text-foreground h-7 gap-1 px-2 text-xs",
        align === "header" && "-mr-2",
        className,
      )}
    >
      <Link to={to} search={search} params={params}>
        {children}
        <ArrowRight className="size-3" />
      </Link>
    </Button>
  );
}
