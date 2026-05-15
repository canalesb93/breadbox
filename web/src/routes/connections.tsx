import { useMemo } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { z } from "zod";
import { Loader2, Plug, Plus, RefreshCw } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { withMutationToast } from "@/lib/mutation-toast";
import { useConnections, useSyncAll } from "@/api/queries/connections";
import { useAccounts } from "@/api/queries/accounts";
import { useUsers } from "@/api/queries/users";
import { ConnectionRow } from "@/features/connections/connection-row";
import { ConnectionsSummary } from "@/features/connections/connections-summary";
import { FamilyTabs } from "@/features/connections/family-tabs";
import { ComingSoonSheet } from "@/features/connections/coming-soon-sheet";
import {
  indexAccountsByConnection,
  needsAttention,
} from "@/features/connections/connection-utils";
import type { Connection } from "@/api/types";

// Search-param schema.
//   user   → "all" or a user short_id  (family-member filter)
//   action → "connect"                 (opens the Connect-bank sheet)
//   reauth → connection short_id        (opens the Re-auth sheet for that row)
// PR-03 / PR-05 wire `action` and `reauth` into real flows. PR-01 only owns
// the URL contract + the placeholder sheet.
export const connectionsSearchSchema = z.object({
  user: z.string().optional(),
  action: z.literal("connect").optional(),
  reauth: z.string().optional(),
});

export type ConnectionsSearch = z.infer<typeof connectionsSearchSchema>;

export function ConnectionsPage() {
  const search = useSearch({ strict: false }) as ConnectionsSearch;
  const navigate = useNavigate();
  const userFilter = search.user ?? "all";

  const connectionsQuery = useConnections();
  const accountsQuery = useAccounts();
  const usersQuery = useUsers();
  const syncAll = useSyncAll();

  const accountStats = useMemo(
    () => indexAccountsByConnection(accountsQuery.data),
    [accountsQuery.data],
  );

  // Connection counts per user, used as a tab-label superscript and as the
  // basis for the visible-after-filtering list below.
  const countsByUser = useMemo(() => {
    const m = new Map<string, number>();
    for (const c of connectionsQuery.data ?? []) {
      // The connection carries the user's UUID; the FamilyTabs key on
      // short_id, which we get by joining against the users list.
      if (!c.user_id) continue;
      m.set(c.user_id, (m.get(c.user_id) ?? 0) + 1);
    }
    return m;
  }, [connectionsQuery.data]);

  // Map UUID counts → short_id counts so the FamilyTabs can render them by
  // the same key it uses for the trigger value.
  const countsByUserShortId = useMemo(() => {
    const out = new Map<string, number>();
    for (const u of usersQuery.data ?? []) {
      out.set(u.short_id, countsByUser.get(u.id) ?? 0);
    }
    return out;
  }, [countsByUser, usersQuery.data]);

  // Resolve the selected short_id back to a UUID for the row filter.
  const filterUserId = useMemo(() => {
    if (userFilter === "all") return null;
    return usersQuery.data?.find((u) => u.short_id === userFilter)?.id ?? null;
  }, [userFilter, usersQuery.data]);

  const visible = useMemo(() => {
    const all = connectionsQuery.data ?? [];
    if (filterUserId == null) return all;
    return all.filter((c) => c.user_id === filterUserId);
  }, [connectionsQuery.data, filterUserId]);

  const attentionCount = useMemo(
    () => (connectionsQuery.data ?? []).filter(needsAttention).length,
    [connectionsQuery.data],
  );

  function setUserFilter(next: string) {
    navigate({
      to: "/connections",
      search: (prev: ConnectionsSearch) => ({
        ...prev,
        user: next === "all" ? undefined : next,
      }),
      replace: true,
    });
  }

  function openConnectSheet() {
    navigate({
      to: "/connections",
      search: (prev: ConnectionsSearch) => ({ ...prev, action: "connect" }),
      replace: false,
    });
  }

  function openReauthSheet(connection: Connection) {
    navigate({
      to: "/connections",
      search: (prev: ConnectionsSearch) => ({
        ...prev,
        reauth: connection.short_id,
      }),
      replace: false,
    });
  }

  function closeSheets() {
    navigate({
      to: "/connections",
      search: (prev: ConnectionsSearch) => ({
        ...prev,
        action: undefined,
        reauth: undefined,
      }),
      replace: true,
    });
  }

  async function onSyncAll() {
    await withMutationToast(() => syncAll.mutateAsync(), {
      success: "Sync queued for every active connection.",
    });
  }

  const isLoading = connectionsQuery.isLoading;
  const isError = connectionsQuery.isError;
  const connections = connectionsQuery.data ?? [];

  return (
    <>
      <PageHeader
        title="Connections"
        description="Banks and CSV imports that feed transactions into Breadbox."
        actions={
          connections.length > 0 ? (
            <>
              <Button
                variant="outline"
                size="sm"
                onClick={onSyncAll}
                disabled={syncAll.isPending}
              >
                {syncAll.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <RefreshCw className="size-4" />
                )}
                Sync all
              </Button>
              <Button size="sm" onClick={openConnectSheet}>
                <Plus className="size-4" />
                Connect bank
              </Button>
            </>
          ) : null
        }
      />

      {connections.length > 0 && (
        <div className="-mt-4 mb-4">
          <ConnectionsSummary connections={connections} />
        </div>
      )}

      {attentionCount > 0 && (
        <Alert variant="default" className="mb-4 border-amber-500/30 bg-amber-500/5">
          <AlertTitle className="text-amber-700 dark:text-amber-400">
            {attentionCount === 1
              ? "1 connection needs attention"
              : `${attentionCount} connections need attention`}
          </AlertTitle>
          <AlertDescription>
            Re-authenticate the rows below to resume syncing.
          </AlertDescription>
        </Alert>
      )}

      {(usersQuery.data?.length ?? 0) > 1 && (
        <div className="mb-4">
          <FamilyTabs
            users={usersQuery.data ?? []}
            value={userFilter}
            onChange={setUserFilter}
            counts={countsByUserShortId}
            totalCount={connections.length}
          />
        </div>
      )}

      {isLoading ? (
        <div className="text-muted-foreground flex items-center justify-center gap-2 py-12 text-sm">
          <Loader2 className="size-4 animate-spin" /> Loading connections…
        </div>
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>Couldn't load connections</AlertTitle>
          <AlertDescription>
            {connectionsQuery.error instanceof Error
              ? connectionsQuery.error.message
              : "Try refreshing the page."}
          </AlertDescription>
        </Alert>
      ) : connections.length === 0 ? (
        <EmptyState
          icon={Plug}
          title="No connections yet"
          description="Connect a bank to start syncing transactions across your household."
          action={
            <Button onClick={openConnectSheet}>
              <Plus className="size-4" />
              Connect a bank
            </Button>
          }
        />
      ) : visible.length === 0 ? (
        <EmptyState
          title="No connections for this filter"
          description="Switch family member or clear the filter to see other connections."
        />
      ) : (
        <div className="flex flex-col gap-3">
          {visible.map((c) => (
            <ConnectionRow
              key={c.id}
              connection={c}
              // /api/v1/accounts exposes connection_id as the parent's
              // short_id (the consistent compact-ID convention), not its UUID.
              stats={accountStats.get(c.short_id)}
              onReauth={openReauthSheet}
            />
          ))}
        </div>
      )}

      <ComingSoonSheet
        open={search.action === "connect"}
        onOpenChange={(open) => {
          if (!open) closeSheets();
        }}
        title="Connect a bank"
        description="Pick a provider and the family member this connection belongs to."
      />
      <ComingSoonSheet
        open={!!search.reauth}
        onOpenChange={(open) => {
          if (!open) closeSheets();
        }}
        title="Re-authenticate"
        description="Reconnect to the bank to resume syncing this connection."
      />
    </>
  );
}
