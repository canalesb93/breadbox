import { Loader2, MoreHorizontal } from "lucide-react";
import * as React from "react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

type Size = "sm" | "xs";

const TRIGGER_BY_SIZE: Record<Size, string> = {
  // size-8 ghost square — the dominant row-actions trigger across
  // connection-row, household-section, tags-table, api-keys-table.
  sm: "text-muted-foreground hover:text-foreground size-8",
  // size-7 ghost square — the tighter cluster variant used inside
  // hero footers + nested list rows (account-links, rule-row,
  // connection-detail hero).
  xs: "text-muted-foreground hover:text-foreground size-7",
};

const ICON_BY_SIZE: Record<Size, string> = {
  sm: "size-4",
  xs: "size-3.5",
};

export interface RowActionsMenuProps {
  /**
   * Accessible label for the trigger button — used as both the
   * `aria-label` and the Tooltip content. Should describe what the
   * menu controls (e.g. "Connection actions", "Actions for {name}").
   */
  label: string;
  /**
   * Menu items. Use the shadcn `<DropdownMenuItem>` /
   * `<DropdownMenuSeparator>` / `<DropdownMenuLabel>` primitives.
   */
  children: React.ReactNode;
  /**
   * Trigger size token.
   * - `sm` (default): size-8 square + size-4 icon. The dominant
   *   row-actions trigger across list tables.
   * - `xs`: size-7 square + size-3.5 icon. For tighter clusters
   *   (hero footers, nested list rows).
   */
  size?: Size;
  /**
   * Content alignment relative to the trigger. Defaults to `end` so
   * menus open from a right-aligned trigger without colliding with
   * row content on the left.
   */
  align?: "start" | "center" | "end";
  /**
   * Optional fixed width on the content. Pass when a menu has long
   * labels that would otherwise collapse the auto-width content
   * (e.g. household-section's `w-52`, rule-row's `w-44`).
   */
  contentClassName?: string;
  /**
   * Trigger disabled flag. While disabled, the button reads
   * non-interactive and the tooltip stays mounted so the row still
   * announces what the menu *would* do.
   */
  disabled?: boolean;
  /**
   * When true, swap the trigger icon for a spinning Loader2 — useful
   * while a row's underlying mutation (reconcile, pause, etc.) is in
   * flight. Implies `disabled`.
   */
  loading?: boolean;
  /**
   * Extra className for the trigger button. Use sparingly — the
   * built-in `size` token covers the canonical shapes. Today only
   * connection-detail's hero footer reaches for this to add
   * `rounded-full` so the kebab pairs visually with the surrounding
   * pill cluster.
   */
  triggerClassName?: string;
  /**
   * Click handler on the trigger. The dominant use case is
   * `e.stopPropagation()` for menus rendered inside clickable list
   * rows (api-keys-table, tags-table) so opening the menu doesn't
   * also navigate to the row's detail page.
   */
  onTriggerClick?: React.MouseEventHandler<HTMLButtonElement>;
  /**
   * Click handler on the content surface. Same `stopPropagation`
   * pattern as `onTriggerClick`, but on the floating content — useful
   * when consumers want the menu items themselves to also stop the
   * row-click handler from firing.
   */
  onContentClick?: React.MouseEventHandler<HTMLDivElement>;
}

/**
 * Canonical row-actions kebab menu — Tooltip + DropdownMenuTrigger +
 * Button lockup that every list row, hero footer, and inline action
 * cluster shares so the trigger geometry, icon glyph, and aria
 * vocabulary stay consistent across the SPA.
 *
 * Sibling of `<ActionPill>` (iter 76) — same `text-muted-foreground →
 * hover:text-foreground` icon-button vocabulary, but ActionPill is a
 * labelled inline action, RowActionsMenu is a kebab that opens a
 * menu.
 */
export function RowActionsMenu({
  label,
  children,
  size = "sm",
  align = "end",
  contentClassName,
  disabled,
  loading,
  triggerClassName,
  onTriggerClick,
  onContentClick,
}: RowActionsMenuProps) {
  const Icon = loading ? Loader2 : MoreHorizontal;
  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              aria-label={label}
              disabled={disabled || loading}
              onClick={onTriggerClick}
              className={cn(TRIGGER_BY_SIZE[size], triggerClassName)}
            >
              <Icon
                className={cn(
                  ICON_BY_SIZE[size],
                  loading && "animate-spin",
                )}
              />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent
        align={align}
        className={contentClassName}
        onClick={onContentClick}
      >
        {children}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
