import { Link } from "@tanstack/react-router";
import { ArrowRight, Building2, FileSpreadsheet, Landmark, Plug } from "lucide-react";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/empty-state";
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
// header's "Manage" link goes to the full Connections page.
export function HomeConnectionsPanel({
  connections,
  isLoading,
}: HomeConnectionsPanelProps) {
  const visible = connections ? rankForDashboard(connections).slice(0, 5) : [];
  const remaining = connections ? Math.max(0, connections.length - visible.length) : 0;

  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b py-4">
        <CardTitle className="text-sm font-medium">Connections</CardTitle>
        <CardAction>
          <Button asChild variant="ghost" size="sm" className="-mr-2 h-8 px-2">
            <Link
              to="/connections"
              className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1 text-xs"
            >
              Manage
              <ArrowRight className="size-3.5" />
            </Link>
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="px-0 py-0">
        {isLoading ? (
          <ConnectionsSkeleton />
        ) : !connections || connections.length === 0 ? (
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
        ) : (
          <>
            <ul className="divide-y">
              {visible.map((c) => (
                <ConnectionRow key={c.id} connection={c} />
              ))}
            </ul>
            {remaining > 0 && (
              <div className="text-muted-foreground border-t px-5 py-3 text-xs">
                +{remaining} more on the Connections page
              </div>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}

function ConnectionRow({ connection: c }: { connection: Connection }) {
  const Icon = PROVIDER_ICON[c.provider] ?? Building2;
  return (
    <li className="flex items-center gap-3 px-5 py-3">
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
    </li>
  );
}

function ConnectionsSkeleton() {
  return (
    <ul className="divide-y">
      {[0, 1, 2, 3].map((i) => (
        <li key={i} className="flex items-center gap-3 px-5 py-3.5">
          <Skeleton className="size-9 rounded-md" />
          <div className="flex-1 space-y-1.5">
            <Skeleton className="h-3.5 w-36" />
            <Skeleton className="h-3 w-20" />
          </div>
          <Skeleton className="h-5 w-16 rounded-md" />
        </li>
      ))}
    </ul>
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
