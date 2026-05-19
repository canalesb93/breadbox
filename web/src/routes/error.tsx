import {
  AlertTriangle,
  ArrowRight,
  ChevronDown,
  Home,
  RotateCw,
} from "lucide-react";
import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Eyebrow } from "@/components/eyebrow";
import { Kbd, KbdGroup } from "@/components/ui/kbd";

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
// Single-card hero (not PageHeader + StatusPanel + SectionCard): the
// previous layout repeated the same destructive message in three stacked
// containers. Now the error message itself IS the heading; the surrounding
// "Something went wrong" framing lives as a small eyebrow above it. The
// recovery actions cluster directly under the message, and the raw stack
// trace lives behind a collapsible disclosure so debugging context is one
// click away without dominating the surface for end users.
export function ErrorPage({ error, reset }: ErrorPageProps) {
  const { message, stack } = readError(error);

  return (
    <div className="mx-auto w-full max-w-2xl">
      <div
        className="border-destructive/30 bg-card relative overflow-hidden rounded-xl border shadow-sm"
        role="alert"
      >
        {/* Destructive accent stripe along the top. Same tone vocabulary as
            <StatusPanel destructive> but a top-rail instead of a left-rail
            so the card reads as the page hero, not an inline notice. */}
        <div className="bg-destructive/80 absolute top-0 right-0 left-0 h-[3px]" />

        <div className="flex flex-col items-center gap-4 px-6 pt-10 pb-8 text-center sm:px-10">
          <div className="bg-destructive/10 text-destructive ring-destructive/20 flex size-14 items-center justify-center rounded-2xl ring-1">
            <AlertTriangle className="size-7" />
          </div>

          <Eyebrow as="p" variant="page" className="text-destructive/80">
            500 · Server error
          </Eyebrow>

          <h1 className="text-foreground text-2xl font-semibold tracking-tight">
            Something went wrong
          </h1>

          <p className="text-muted-foreground max-w-md text-sm leading-relaxed">
            The page hit an unexpected error while rendering. The shell is
            still healthy — try again, or jump to another page.
          </p>

          {/* The raw error message in monospace so it stands apart from the
              copy. Selectable + wraps long messages cleanly. */}
          <code className="bg-muted/60 text-foreground border-border/60 mt-1 max-w-full overflow-x-auto rounded-md border px-3 py-2 text-left font-mono text-xs leading-relaxed break-words whitespace-pre-wrap">
            {message}
          </code>

          <div className="mt-4 flex flex-wrap items-center justify-center gap-2">
            {reset ? (
              <Button size="sm" onClick={reset} className="gap-1.5">
                <RotateCw className="size-3.5" />
                Try again
              </Button>
            ) : null}
            <Button
              variant="outline"
              size="sm"
              onClick={() => window.location.reload()}
              className="gap-1.5"
            >
              <RotateCw className="size-3.5" />
              Reload page
            </Button>
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
            <Button asChild variant="outline" size="sm" className="gap-1.5">
              <Link to="/">
                <Home className="size-3.5" />
                Home
                <ArrowRight className="size-3.5" />
              </Link>
            </Button>
          </div>
        </div>

        {stack ? (
          <Collapsible>
            <div className="border-t">
              <CollapsibleTrigger asChild>
                <button
                  type="button"
                  className="group hover:bg-muted/30 flex w-full items-center justify-between gap-2 px-6 py-3 text-left transition-colors sm:px-10"
                >
                  <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
                    Technical details
                  </span>
                  <ChevronDown className="text-muted-foreground size-3.5 transition-transform group-data-[state=open]:rotate-180" />
                </button>
              </CollapsibleTrigger>
              <CollapsibleContent>
                <div className="px-6 pb-6 sm:px-10">
                  <pre className="bg-muted/40 text-muted-foreground max-h-[440px] overflow-auto overscroll-contain rounded-md border p-3 text-[11px] leading-relaxed [-webkit-overflow-scrolling:touch]">
                    {stack}
                  </pre>
                </div>
              </CollapsibleContent>
            </div>
          </Collapsible>
        ) : null}
      </div>
    </div>
  );
}
