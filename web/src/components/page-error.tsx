import { AlertTriangle, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
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
   * - `panel` (default): centred card with a destructive icon tile, heading,
   *   body, and a retry CTA beneath. Use for top-level page errors that
   *   own the whole route body.
   * - `inline`: chrome-less variant for nesting inside an existing bordered
   *   host (a SectionCard body, a ListCard slot). Keeps the destructive tile
   *   + heading + body + retry vocabulary, but drops the panel border so two
   *   nested borders don't read heavy.
   */
  variant?: PageErrorVariant;
  className?: string;
}

// PageError renders the canonical page-level "this page couldn't fetch its
// data" state — destructive-toned card with an AlertTriangle icon, a concrete
// heading, the error message (or a fallback), and an optional Retry button.
//
// The `panel` variant is shaped like an empty-state hero (centred,
// generous padding, dashed border) but destructive-toned — a clear signal
// that the surface is in a failure state, not just empty. The `inline`
// variant strips the border so it can sit inside an already-bordered host
// without doubling up chrome.
//
// Sibling of `<EmptyState>` (no-data) and `<DetailPageSkeleton>` (loading).
// Don't fork — extend this primitive.
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
      <div className={cn("flex items-start gap-3", className)}>
        <span className="bg-destructive/10 text-destructive flex size-9 shrink-0 items-center justify-center rounded-xl">
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
    <div
      className={cn(
        "border-destructive/25 bg-destructive/[0.02] flex flex-col items-center justify-center gap-2 rounded-lg border border-dashed px-6 py-10 text-center",
        className,
      )}
      role="alert"
    >
      <div className="bg-destructive/10 text-destructive mb-2 flex size-11 items-center justify-center rounded-xl">
        <AlertTriangle className="size-5" />
      </div>
      <h3 className="text-foreground text-sm font-medium">
        {`Couldn't load ${resource}`}
      </h3>
      <p className="text-muted-foreground max-w-sm text-sm">{message}</p>
      {retryButton && <div className="mt-3">{retryButton}</div>}
    </div>
  );
}
