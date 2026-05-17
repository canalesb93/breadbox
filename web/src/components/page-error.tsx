import { AlertTriangle, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { StatusPanel } from "@/components/status-panel";

interface PageErrorProps {
  /** What couldn't load (rendered as `Couldn't load {resource}.`). */
  resource: string;
  /** Underlying error to surface; `Error.message` is used as the body. */
  error?: unknown;
  /** Optional fetcher to invoke when the user presses Retry. */
  onRetry?: () => unknown | Promise<unknown>;
  /** Whether the retry call is currently in-flight. */
  retrying?: boolean;
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
// The component reuses the StatusPanel vocabulary (3px tone-tinted left rail
// + tinted icon tile + heading + body + trailing slot) so destructive page
// errors speak the same language as warnings, success notices, and env-locked
// states (see iter 16). The retry button lands in StatusPanel's `trailing`
// slot so it sits on the right edge, aligned with the icon row.
export function PageError({
  resource,
  error,
  onRetry,
  retrying = false,
}: PageErrorProps) {
  const message =
    error instanceof Error && error.message
      ? error.message
      : "Try again or refresh the page.";

  return (
    <StatusPanel
      tone="destructive"
      icon={AlertTriangle}
      heading={`Couldn't load ${resource}`}
      body={message}
      trailing={
        onRetry ? (
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
        ) : undefined
      }
    />
  );
}
