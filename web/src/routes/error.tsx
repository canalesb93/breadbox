import { useState } from "react";
import {
  AlertTriangle,
  ArrowRight,
  ChevronDown,
  ChevronRight,
  RotateCw,
} from "lucide-react";
import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/page-header";
import { SectionCard } from "@/components/section-card";
import { StatusPanel } from "@/components/status-panel";
import { Kbd, KbdGroup } from "@/components/ui/kbd";
import { cn } from "@/lib/utils";

interface ErrorPageProps {
  /** The error thrown during render — typed loosely to match TanStack
   *  Router's `ErrorComponent` props (`error: unknown`). */
  error?: unknown;
  /** Reset handler from the router. When invoked, the router re-renders the
   *  failing route as if it were a fresh mount. Optional so the same
   *  component can be used standalone (where reload is the recovery). */
  reset?: () => void;
}

function openCommandPalette() {
  window.dispatchEvent(new CustomEvent("breadbox:command-palette:open"));
}

// Extract a human-readable message + an optional stack from any thrown
// value. Strings are common (`throw "boom"`), Error instances even more so,
// but ApiError and similar carry useful extra context.
function readError(err: unknown): { message: string; stack?: string } {
  if (err instanceof Error) {
    return { message: err.message, stack: err.stack };
  }
  if (typeof err === "string") return { message: err };
  if (err && typeof err === "object" && "message" in err) {
    const message = (err as { message?: unknown }).message;
    if (typeof message === "string") return { message };
  }
  return { message: "An unexpected error occurred." };
}

// ErrorPage is the canonical error-boundary surface for the v2 SPA. Wired
// into `createRouter({ defaultErrorComponent })` so it renders in place of
// `<Outlet/>` whenever a route throws during render — the sidebar, topbar,
// and command palette stay live, so the user has a way out without a hard
// reload.
//
// Visual contract (matches NotFoundPage / Placeholder):
//   <PageHeader eyebrow="500 · ERROR" title="Something went wrong" />
//   <StatusPanel tone="destructive" />  ← human-readable message
//   <SectionCard title="Technical details">
//     collapsible details payload with the raw stack trace (dev affordance)
//   </SectionCard>
//
// Reset wiring: the router passes a `reset` callback that re-renders the
// failing route. We expose it as "Try again" alongside a "Reload" fallback
// (full-page) and a Home link.
export function ErrorPage({ error, reset }: ErrorPageProps) {
  const [showDetails, setShowDetails] = useState(false);
  const { message, stack } = readError(error);

  return (
    <>
      <PageHeader
        eyebrow="500 · ERROR"
        title="Something went wrong"
        description="The page hit an unexpected error while rendering. The shell is still healthy — try again, or jump to another page."
        actions={
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={openCommandPalette}
              className="gap-2"
            >
              <span>Jump to…</span>
              <KbdGroup>
                <Kbd>⌘</Kbd>
                <Kbd>K</Kbd>
              </KbdGroup>
            </Button>
            {reset ? (
              <Button
                variant="outline"
                size="sm"
                onClick={reset}
                className="gap-1.5"
              >
                <RotateCw className="size-3.5" />
                Try again
              </Button>
            ) : null}
            <Button asChild size="sm">
              <Link to="/">
                Back to Home
                <ArrowRight className="size-4" />
              </Link>
            </Button>
          </>
        }
      />

      <div className="flex flex-col gap-4">
        <StatusPanel
          tone="destructive"
          icon={AlertTriangle}
          heading={message}
          body="If this keeps happening, reload the page or check the browser console for more context."
          trailing={
            <Button
              variant="ghost"
              size="sm"
              onClick={() => window.location.reload()}
              className="h-7 gap-1.5 px-2.5 text-xs"
            >
              <RotateCw className="size-3" />
              Reload
            </Button>
          }
        />

        {stack ? (
          <SectionCard
            title="Technical details"
            icon={
              <AlertTriangle className="text-muted-foreground size-4" />
            }
            action={
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setShowDetails((v) => !v)}
                className="h-7 gap-1 px-2 text-xs"
              >
                {showDetails ? (
                  <ChevronDown className="size-3.5" />
                ) : (
                  <ChevronRight className="size-3.5" />
                )}
                {showDetails ? "Hide" : "Show"}
              </Button>
            }
          >
            <div
              className={cn(
                "overflow-hidden transition-all",
                showDetails ? "max-h-[480px]" : "max-h-0",
              )}
            >
              <pre className="bg-muted/40 text-muted-foreground max-h-[440px] overflow-auto rounded-md border p-3 text-[11px] leading-relaxed">
                {stack}
              </pre>
            </div>
          </SectionCard>
        ) : null}
      </div>
    </>
  );
}
