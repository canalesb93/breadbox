import { AlertTriangle, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { StatusPanel } from "@/components/status-panel";
import { cn } from "@/lib/utils";

export type PageErrorVariant = "panel" | "inline";

interface PageErrorProps {
  /** What couldn't load (rendered as `Couldn't load {resource}.`). */
  resource: string;
  /** Underlying error to surface; `Error.message` is used as the body. */
  error?: unknown;
  /** Optional fetcher to invoke when the user presses Retry. */
  onRetry?: () => unknown | Promise<unknown>;
  /** Whether the retry call is currently in-flight. */
  retrying?: boolean;
  /**
   * Visual shell.
   * - `panel` (default): bordered StatusPanel with the destructive rail.
   *   Use for top-level page errors that own the whole route body.
   * - `inline`: chrome-less variant for nesting inside an existing bordered
   *   host (a SectionCard body, a ListCard slot). Keeps the destructive icon
   *   tile + heading + body + retry vocabulary, but drops the panel border +
   *   rail so two nested borders don't read heavy.
   */
  variant?: PageErrorVariant;
  className?: string;
}

// PageError renders the canonical page-level "this page couldn't fetch its
// data" state — destructive-toned StatusPanel with an AlertTriangle icon, a
// concrete heading, the error message (or a fallback), and an optional
// inline Retry button.
//
// Until iter 82 every page hand-rolled its own `<Alert variant="destructive">
// + AlertTitle + AlertDescription` for this state. Six pages now route
// through this primitive: accounts, connections, providers, rules, rule-form,
// rule-detail. Don't fork — extend this primitive if a seventh consumer
// needs a new variant.
//
// The `panel` variant reuses the StatusPanel vocabulary (3px tone-tinted left
// rail + tinted icon tile + heading + body + trailing slot) so destructive
// page errors speak the same language as warnings, success notices, and
// env-locked states (see iter 16). The retry button lands in StatusPanel's
// `trailing` slot so it sits on the right edge, aligned with the icon row.
//
// The `inline` variant (iter 88) drops the panel chrome — same icon tile +
// heading + body + retry, but no border / rail / muted background. Used by
// the activity-timeline inside a SectionCard, where nesting a second
// bordered destructive panel inside the section's bordered card read heavy.
// Two nested borders → one tile + text block aligned with the rest of the
// section body.
export function PageError({
  resource,
  error,
  onRetry,
  retrying = false,
  variant = "panel",
  className,
}: PageErrorProps) {
  const message =
    error instanceof Error && error.message
      ? error.message
      : "Try again or refresh the page.";

  const retryButton = onRetry ? (
    <Button
      type="button"
      size="sm"
      variant="outline"
      onClick={() => {
        void onRetry();
      }}
      disabled={retrying}
    >
      <RefreshCw
        className={retrying ? "size-3.5 animate-spin" : "size-3.5"}
      />
      {retrying ? "Retrying…" : "Retry"}
    </Button>
  ) : null;

  if (variant === "inline") {
    return (
      <div
        className={cn(
          "flex items-start gap-3",
          className,
        )}
      >
        <span className="bg-destructive/10 text-destructive flex size-8 shrink-0 items-center justify-center rounded-md">
          <AlertTriangle className="size-4" />
        </span>
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="text-foreground text-sm font-medium">
            {`Couldn't load ${resource}`}
          </span>
          <span className="text-muted-foreground text-xs leading-relaxed">
            {message}
          </span>
        </div>
        {retryButton && (
          <div className="ml-2 flex shrink-0 items-center">{retryButton}</div>
        )}
      </div>
    );
  }

  return (
    <StatusPanel
      tone="destructive"
      icon={AlertTriangle}
      heading={`Couldn't load ${resource}`}
      body={message}
      trailing={retryButton ?? undefined}
      className={className}
    />
  );
}
