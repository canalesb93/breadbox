import { cn } from "@/lib/utils";

export interface FloatingActionBarProps {
  /** Pill contents — buttons, text, separators. The bar is fully
   *  agnostic about what goes inside; caller composes. */
  children: React.ReactNode;
  className?: string;
  /** Per-instance label for assistive tech (e.g. "Bulk transaction
   *  actions", "Prompt builder actions"). */
  ariaLabel?: string;
}

// FloatingActionBar is the shared floating-pill chrome used by
// surfaces that want a viewport-pinned action toolbar — bulk
// selection on /v2/transactions, output actions on /v2/prompts/build,
// any future "act on the page" surface. Outer wrapper is
// `pointer-events-none` so the empty band stays click-through; only
// the inner pill captures events. Use this for *transient or
// contextual* actions; PageHeader.actions is still the right home for
// persistent page-level affordances (create, settings).
export function FloatingActionBar({
  children,
  className,
  ariaLabel,
}: FloatingActionBarProps) {
  return (
    <div
      className="pointer-events-none fixed inset-x-0 bottom-4 z-40 flex justify-center px-4 sm:bottom-6"
      role={ariaLabel ? "toolbar" : undefined}
      aria-label={ariaLabel}
    >
      <div
        data-state="open"
        className={cn(
          "pointer-events-auto bg-popover/95 text-popover-foreground flex max-w-[calc(100dvw-2rem)] items-center gap-1 overflow-hidden rounded-full border p-1 shadow-xl backdrop-blur-sm",
          "data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:slide-in-from-bottom-4 data-[state=open]:duration-200",
          className,
        )}
      >
        {children}
      </div>
    </div>
  );
}
