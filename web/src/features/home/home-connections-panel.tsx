import { Link } from "@tanstack/react-router";
import { ArrowRight, Building2, FileSpreadsheet, Landmark, Plug } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/empty-state";
import { ListCard } from "@/components/list-card";
import { ConnectionStatusBadge } from "@/features/connections/connection-status-badge";
import { relativeTime } from "@/features/connections/connection-utils";
import { cn } from "@/lib/utils";
import type { Connection } from "@/api/types";

interface HomeConnectionsPanelProps {
  connections: Connection[] | undefined;
  isLoading: boolean;
}

const PROVIDER_ICON: Record<string, typeof Building2> = {
  plaid: Building2,
  teller: Landmark,
  csv: FileSpreadsheet,
};

// Side-rail summary of household connections. Sorts attention-needed ones to
// the top so the user sees what to fix first. Caps at 5 rows; the card
// header's "Manage" link goes to the full Connections page. The bordered
// header carries the household-level health summary (active/needs-action
// count) so the page header doesn't need a fourth scoreboard tile for the
// same number.
export function HomeConnectionsPanel({
  connections,
  isLoading,
}: HomeConnectionsPanelProps) {
  const visible = connections ? rankForDashboard(connections).slice(0, 5) : [];
  const remaining = connections ? Math.max(0, connections.length - visible.length) : 0;
  const attention = (connections ?? []).filter(
    (c) => c.status === "pending_reauth" || c.status === "error",
  ).length;
  const active = (connections ?? []).filter((c) => c.status === "active").length;

  const manage = (
    <Button asChild variant="ghost" size="sm" className="-mr-2 h-8 px-2">
      <Link
        to="/connections"
        className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1 text-xs"
      >
        Manage
        <ArrowRight className="size-3.5" />
      </Link>
    </Button>
  );

  // Health line appears in the header subtitle when we have any connections.
  // It replaces the standalone "Connections" stat card in the hero strip —
  // the same number, but inline next to where the user is about to read it.
  const title =
    !isLoading && connections && connections.length > 0 ? (
      <span className="flex flex-col gap-0.5">
        <span>Connections</span>
        <span className="text-muted-foreground text-xs font-normal">
          {attention > 0 ? (
            <>
              <span className="text-amber-600 dark:text-amber-400">
                {attention} need{attention === 1 ? "s" : ""} action
              </span>
              <span> · {active} healthy</span>
            </>
          ) : (
            <>{active} healthy</>
          )}
        </span>
      </span>
    ) : (
      "Connections"
    );

  if (isLoading) {
    return (
      <ListCard
        title={title}
        action={manage}
        rows={[0, 1, 2, 3]}
        getRowKey={(i) => i}
        renderRow={(i) => (
          <div className="flex items-center gap-3 px-5 py-3.5" key={i}>
            <Skeleton className="size-9 rounded-md" />
            <div className="flex-1 space-y-1.5">
              <Skeleton className="h-3.5 w-36" />
              <Skeleton className="h-3 w-20" />
            </div>
            <Skeleton className="h-5 w-16 rounded-md" />
          </div>
        )}
      />
    );
  }

  if (!connections || connections.length === 0) {
    return (
      <ListCard
        title={title}
        action={manage}
        rows={[]}
        renderRow={() => null}
        empty={
          <EmptyState
            icon={Plug}
            title="No banks connected"
            description="Link a bank or upload a CSV to start syncing transactions."
            action={
              <Button asChild size="sm">
                <Link to="/connections" search={{ action: "connect" }}>
                  Connect a bank
                </Link>
              </Button>
            }
          />
        }
      />
    );
  }

  return (
    <ListCard
      title={title}
      action={manage}
      rows={visible}
      getRowKey={(c) => c.id}
      renderRow={(c) => <ConnectionRow connection={c} />}
      footer={
        remaining > 0 ? (
          <span className="text-muted-foreground text-xs">
            +{remaining} more on the Connections page
          </span>
        ) : undefined
      }
      footerClassName="text-left"
    />
  );
}

function ConnectionRow({ connection: c }: { connection: Connection }) {
  const Icon = PROVIDER_ICON[c.provider] ?? Building2;
  return (
    <div className="flex items-center gap-3 px-5 py-3">
      <div
        className={cn(
          "flex size-9 shrink-0 items-center justify-center rounded-md border",
          "bg-muted/40 text-muted-foreground",
        )}
      >
        <Icon className="size-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">
          {c.institution_name || "Untitled connection"}
        </div>
        <div className="text-muted-foreground mt-0.5 truncate text-xs">
          Synced {relativeTime(c.last_synced_at)}
        </div>
      </div>
      <ConnectionStatusBadge status={c.status} />
    </div>
  );
}

// Attention first (pending_reauth, error), then active, then disconnected.
// Within each bucket, most-recently-synced first.
function rankForDashboard(connections: Connection[]): Connection[] {
  const score = (c: Connection): number => {
    if (c.status === "pending_reauth" || c.status === "error") return 0;
    if (c.status === "active") return 1;
    return 2;
  };
  return [...connections].sort((a, b) => {
    const s = score(a) - score(b);
    if (s !== 0) return s;
    const ta = a.last_synced_at ? new Date(a.last_synced_at).getTime() : 0;
    const tb = b.last_synced_at ? new Date(b.last_synced_at).getTime() : 0;
    return tb - ta;
  });
}
