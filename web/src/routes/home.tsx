import { useMemo } from "react";
import { Link } from "@tanstack/react-router";
import { ArrowUpRight, Plus } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { useMe } from "@/api/queries/me";
import { useAccounts } from "@/api/queries/accounts";
import { useConnections } from "@/api/queries/connections";
import { useTransactions } from "@/api/queries/transactions";
import { relativeTime } from "@/features/connections/connection-utils";
import { HomeStats } from "@/features/home/home-stats";
import { HomeRecentTransactions } from "@/features/home/home-recent-transactions";
import { HomeConnectionsPanel } from "@/features/home/home-connections-panel";

// Greeting strips the email domain so "admin@example.com" reads as "admin".
// Username can already be a display name on real installs, in which case it
// passes through unchanged.
function greetingName(username: string | undefined): string {
  if (!username) return "";
  const at = username.indexOf("@");
  return at === -1 ? username : username.slice(0, at);
}

export function HomePage() {
  const { data: me } = useMe();
  const accountsQuery = useAccounts();
  const connectionsQuery = useConnections();
  // Default sort = date desc; cap to one page worth and only render the top
  // few. Reuses the same cache key the Transactions list uses for an empty
  // filter set, so navigating to /transactions hits a warm cache.
  const txQuery = useTransactions({});

  const recent = useMemo(
    () => txQuery.data?.pages?.[0]?.transactions?.slice(0, 6),
    [txQuery.data],
  );

  // The freshest `last_synced_at` across all connections is the household-level
  // "synced X ago" stamp — matches what the v1 dashboard's header showed and
  // gives the page a single trustworthy freshness signal.
  const lastSyncedAt = useMemo(() => {
    const stamps = (connectionsQuery.data ?? [])
      .map((c) => c.last_synced_at)
      .filter((s): s is string => !!s)
      .map((s) => new Date(s).getTime());
    if (stamps.length === 0) return null;
    return new Date(Math.max(...stamps)).toISOString();
  }, [connectionsQuery.data]);

  const isLoadingShell =
    accountsQuery.isLoading || connectionsQuery.isLoading;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        eyebrow={lastSyncedAt ? `Synced ${relativeTime(lastSyncedAt)}` : "Overview"}
        title={me ? `Welcome back, ${greetingName(me.username)}` : "Welcome"}
        description="A quick read on balances, recent activity, and the health of your bank connections."
        actions={
          <>
            <Button asChild variant="outline" size="sm">
              <Link to="/transactions" className="inline-flex items-center gap-1.5">
                <ArrowUpRight className="size-4" />
                Transactions
              </Link>
            </Button>
            <Button asChild size="sm">
              <Link
                to="/connections"
                search={{ action: "connect" }}
                className="inline-flex items-center gap-1.5"
              >
                <Plus className="size-4" />
                Connect bank
              </Link>
            </Button>
          </>
        }
      />

      <HomeStats
        accounts={accountsQuery.data}
        connections={connectionsQuery.data}
        isLoading={isLoadingShell}
      />

      <div className="grid gap-6 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <HomeRecentTransactions
            transactions={recent}
            isLoading={txQuery.isLoading}
          />
        </div>
        <div className="lg:col-span-1">
          <HomeConnectionsPanel
            connections={connectionsQuery.data}
            isLoading={connectionsQuery.isLoading}
          />
        </div>
      </div>
    </div>
  );
}
