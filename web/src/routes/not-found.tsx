import { ArrowRight, Compass, FileQuestion } from "lucide-react";
import { Link, useRouterState } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/page-header";
import { SectionCard } from "@/components/section-card";
import { StatusPanel } from "@/components/status-panel";
import { IdPill } from "@/components/id-pill";
import { Kbd, KbdGroup } from "@/components/ui/kbd";

// Suggested jumps for a not-found page. Pulled out as a flat list so the
// hero stays scannable — these are the top entry points any wandering link
// likely meant to land on.
const QUICK_JUMPS: Array<{ title: string; description: string; to: string }> = [
  {
    title: "Home",
    description: "Dashboard, recent activity, and attention items.",
    to: "/",
  },
  {
    title: "Transactions",
    description: "All transactions across every account.",
    to: "/transactions",
  },
  {
    title: "Accounts",
    description: "Banks, cards, and balances by household member.",
    to: "/accounts",
  },
  {
    title: "Categories",
    description: "Category tree, rules, and category settings.",
    to: "/categories",
  },
];

function openCommandPalette() {
  window.dispatchEvent(new CustomEvent("breadbox:command-palette:open"));
}

// NotFoundPage is the canonical 404 surface for the v2 SPA. Wired into
// `createRouter({ defaultNotFoundComponent })` so it renders in place of
// `<Outlet/>` inside the authenticated shell — the sidebar, topbar, and
// command palette stay live, so the user has a way out without a hard
// reload.
//
// Visual contract (matches the rest of v2):
//   <PageHeader eyebrow="404 · NOT FOUND" title="Page not found" description />
//   <StatusPanel tone="info" />  ← shows the attempted path as an IdPill
//   <SectionCard title="Jump to a page">
//     2x2 grid of quick-jump tiles with arrow affordance
//   </SectionCard>
//
// Reuses the same StatusPanel + SectionCard + IdPill vocabulary as the
// Placeholder page so the two "you've landed somewhere unexpected" surfaces
// feel like siblings.
export function NotFoundPage() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  return (
    <>
      <PageHeader
        eyebrow="404 · NOT FOUND"
        title="Page not found"
        description="The path you followed doesn't match any page in the v2 admin. It may have moved, been renamed, or never existed."
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
          tone="info"
          icon={FileQuestion}
          heading="No route matched this URL"
          body={
            <span className="flex flex-wrap items-center gap-2">
              <span>Attempted path:</span>
              <IdPill value={pathname} />
            </span>
          }
        />

        <SectionCard
          title="Jump to a page"
          icon={<Compass className="text-muted-foreground size-4" />}
        >
          <ol className="grid gap-3 sm:grid-cols-2">
            {QUICK_JUMPS.map((jump) => (
              <li key={jump.to}>
                <Link
                  to={jump.to}
                  className="group bg-muted/20 hover:bg-muted/40 hover:border-ring/40 relative flex h-full items-center justify-between gap-3 rounded-md border p-4 transition-colors"
                >
                  <div className="space-y-1">
                    <h3 className="text-foreground text-sm font-medium">
                      {jump.title}
                    </h3>
                    <p className="text-muted-foreground text-xs leading-relaxed">
                      {jump.description}
                    </p>
                  </div>
                  <ArrowRight className="text-muted-foreground/50 group-hover:text-muted-foreground size-4 shrink-0 transition-colors" />
                </Link>
              </li>
            ))}
          </ol>
        </SectionCard>
      </div>
    </>
  );
}
