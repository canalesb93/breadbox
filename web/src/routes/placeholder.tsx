import {
  ArrowRight,
  FileText,
  Hammer,
  type LucideIcon,
  Sparkles,
} from "lucide-react";
import { Link, useRouterState } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { ComingSoonPill } from "@/components/coming-soon-pill";
import { JumpToPill } from "@/components/jump-to-pill";
import { PageHeader } from "@/components/page-header";
import { SectionCard } from "@/components/section-card";
import { StatusPanel } from "@/components/status-panel";
import { Kbd, KbdGroup } from "@/components/ui/kbd";
import { NAV_LEAVES } from "@/lib/nav";

interface PlannedFeature {
  title: string;
  description: string;
}

interface RelatedLink {
  title: string;
  to: string;
}

interface PlaceholderContent {
  description: string;
  features: PlannedFeature[];
  related: RelatedLink[];
}

// Per-route copy for the "coming soon" placeholder. Add an entry here when a
// new nav leaf lands without a real implementation — the route is wired up
// in main.tsx via NAV_LEAVES; this just makes the landing surface feel
// intentional instead of a generic "page is empty" plate.
const CONTENT: Record<string, PlaceholderContent> = {
  "/reports": {
    description:
      "Insights you can act on — cashflow trends, category breakdowns, and exportable reports built on top of your transaction history.",
    features: [
      {
        title: "Cashflow over time",
        description:
          "Monthly inflow vs outflow with a running balance line, scoped by account or household member.",
      },
      {
        title: "Category breakdown",
        description:
          "See where money goes by category and sub-category — drill into any slice to land back on the transactions list, pre-filtered.",
      },
      {
        title: "Saved views & exports",
        description:
          "Pin the cuts you check most often, share read-only links, and export to CSV for spreadsheets or downstream tools.",
      },
    ],
    related: [
      { title: "Transactions", to: "/transactions" },
      { title: "Categories", to: "/categories" },
      { title: "Accounts", to: "/accounts" },
    ],
  },
  "/agents": {
    description:
      "A first-class view of the AI agents connected to your household — what they can see, what they've done, and where to revoke access.",
    features: [
      {
        title: "Connected agents",
        description:
          "List the MCP clients that have called Breadbox, with first-seen / last-seen and the audit-session count per client.",
      },
      {
        title: "Activity timeline",
        description:
          "Per-agent feed of recent tool calls, threaded under audit sessions so you can read a conversation as a single unit.",
      },
      {
        title: "Scope controls",
        description:
          "Promote a key from read-only to read-write, revoke a session, or pin a rate-limit — without leaving the page.",
      },
    ],
    related: [
      { title: "API keys", to: "/api-keys" },
      { title: "Connections", to: "/connections" },
    ],
  },
};

const DEFAULT_CONTENT: PlaceholderContent = {
  description:
    "This page is part of the v2 admin shell. The full implementation lands in a follow-up PR.",
  features: [],
  related: [],
};

function openCommandPalette() {
  window.dispatchEvent(new CustomEvent("breadbox:command-palette:open"));
}

// Placeholder is the canonical "page not yet implemented" surface used by
// main.tsx for any nav leaf without a PAGE_OVERRIDES entry (today: Reports
// and Agents). It is NOT an empty-state — `<EmptyState>` (in
// `components/empty-state.tsx`) is for a real page that loaded but has
// nothing to show right now. Placeholder reads as "this page is being
// built; here's what it will cover and where to go in the meantime."
//
// Visual contract (matches the rest of v2):
//   <PageHeader eyebrow={navGroup.toUpperCase()} title={…} description={…} />
//   <StatusPanel tone="info" />  ← "Coming soon" notice with planned-for hint
//   <SectionCard title="What's coming">
//     planned-feature list (icon + title + description)
//     footer slot with related-page links + feature-request link
//   </SectionCard>
//
// Per-route copy lives in CONTENT above. The lookup is keyed off pathname,
// derived from useRouterState, so callers only pass `title` and we resolve
// the rest automatically.
export function Placeholder({ title }: { title: string }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const leafEntry = NAV_LEAVES.find(
    ({ leaf }) => leaf.kind === "link" && leaf.to === pathname,
  );
  const eyebrow = leafEntry ? leafEntry.group.toUpperCase() : undefined;
  const LeafIcon: LucideIcon | undefined = leafEntry?.leaf.icon;
  const content = CONTENT[pathname] ?? DEFAULT_CONTENT;
  const hasPlan = content.features.length > 0;

  return (
    <>
      <PageHeader
        eyebrow={eyebrow}
        title={title}
        description={content.description}
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
          icon={Hammer}
          heading={`${title} is in the works`}
          body={
            hasPlan
              ? "We're still building this surface. The plan below sketches the shape it'll take — follow the related pages for the data that's live today."
              : "This page is part of the v2 admin shell. The full implementation lands in a follow-up PR."
          }
          trailing={<ComingSoonPill />}
        />

        {hasPlan && (
          <SectionCard
            title="What's coming"
            icon={<Sparkles className="text-muted-foreground size-4" />}
            footer={
              content.related.length > 0 ? (
                <div className="flex flex-wrap items-center justify-between gap-3 text-left">
                  <div className="flex items-center gap-2">
                    <span className="text-muted-foreground text-xs">
                      Available today:
                    </span>
                    <div className="flex flex-wrap items-center gap-1.5">
                      {content.related.map((r) => (
                        <JumpToPill key={r.to} asChild>
                          <Link to={r.to}>
                            {r.title}
                            <ArrowRight className="size-3" />
                          </Link>
                        </JumpToPill>
                      ))}
                    </div>
                  </div>
                  <a
                    href="https://github.com/canalesb93/breadbox/issues"
                    target="_blank"
                    rel="noreferrer"
                    className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1.5 text-xs underline-offset-4 hover:underline"
                  >
                    <FileText className="size-3" />
                    Request a feature
                  </a>
                </div>
              ) : undefined
            }
          >
            <ol className="grid gap-4 sm:grid-cols-3">
              {content.features.map((feature, idx) => (
                <li
                  key={feature.title}
                  className="bg-muted/20 relative flex flex-col gap-2 overflow-hidden rounded-md border p-4"
                >
                  <div className="flex items-center gap-2">
                    <span className="bg-background text-muted-foreground flex size-7 items-center justify-center rounded-md border font-mono text-[11px] font-medium tabular-nums">
                      {String(idx + 1).padStart(2, "0")}
                    </span>
                    {LeafIcon && (
                      <LeafIcon className="text-muted-foreground/60 size-3.5" />
                    )}
                  </div>
                  <div className="space-y-1">
                    <h3 className="text-foreground text-sm font-medium">
                      {feature.title}
                    </h3>
                    <p className="text-muted-foreground text-xs leading-relaxed">
                      {feature.description}
                    </p>
                  </div>
                </li>
              ))}
            </ol>
          </SectionCard>
        )}
      </div>
    </>
  );
}
