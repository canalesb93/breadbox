import { useCallback, useMemo, useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { z } from "zod";
import { Loader2, Plug, Plus, RefreshCw } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { ListCard } from "@/components/list-card";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { withMutationToast } from "@/lib/mutation-toast";
import { useShortcut } from "@/lib/shortcuts";
import { useConnections, useSyncAll } from "@/api/queries/connections";
import { useAccounts } from "@/api/queries/accounts";
import { useUsers } from "@/api/queries/users";
import { ConnectionRow } from "@/features/connections/connection-row";
import { ConnectionsSummary } from "@/features/connections/connections-summary";
import { FamilyTabs } from "@/features/connections/family-tabs";
import { ConnectBankSheet } from "@/features/connections/connect-bank-sheet";
import { ReauthSheet } from "@/features/connections/reauth-sheet";
import { SelectionActionBar } from "@/features/connections/selection-action-bar";
import {
  indexAccountsByConnection,
  needsAttention,
} from "@/features/connections/connection-utils";
import type { Connection } from "@/api/types";

// Search-param schema.
//   user   → "all" or a user short_id  (family-member filter)
//   action → "connect"                 (opens the Connect-bank sheet)
//   reauth → connection short_id        (opens the Re-auth sheet for that row)
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

  // Multi-select state — Set of UUIDs (Connection.id, not short_id). Lifted
  // here so the action bar reads the union and the rows pass selection back
  // up. Ephemeral by design: not stored in URL search, cleared after a
  // successful bulk mutation or on family-tab switch.
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());

  // Selected connections that are still visible after filtering — used by the
  // action bar. Filtering also drops any stale ids that may have been removed
  // from the underlying data (e.g. after a bulk disconnect refetch).
  const selectedConnections = useMemo(
    () => visible.filter((c) => selectedIds.has(c.id)),
    [visible, selectedIds],
  );

  const allVisibleSelected =
    visible.length > 0 && selectedConnections.length === visible.length;
  const someVisibleSelected =
    selectedConnections.length > 0 && !allVisibleSelected;

  const toggleRowSelection = useCallback((id: string, next: boolean) => {
    setSelectedIds((prev) => {
      const out = new Set(prev);
      if (next) out.add(id);
      else out.delete(id);
      return out;
    });
  }, []);

  const toggleSelectAll = useCallback(() => {
    setSelectedIds((prev) => {
      if (prev.size > 0) return new Set();
      return new Set(visible.map((c) => c.id));
    });
  }, [visible]);

  const clearSelection = useCallback(() => setSelectedIds(new Set()), []);

  // Esc clears selection. Mirrors the transactions page convention so the
  // same muscle memory works here. Always-registered (cheap) — the handler
  // gates on having something to clear, which avoids the registry churn from
  // toggling `enabled` on every selection change.
  useShortcut(
    ["escape"],
    () => {
      if (selectedIds.size > 0) clearSelection();
    },
    { label: "Clear selection", group: "Connections" },
  );

  function setUserFilter(next: string) {
    // Selection is per-visible-set; switching tabs would otherwise carry
    // hidden rows along silently.
    setSelectedIds(new Set());
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

  // Eyebrow tracks the same vocabulary as the other v2 list pages (Transactions,
  // Tags, Categories): N entities, then Showing N of M when a filter narrows
  // the view. Empty / loading / error get their own short string.
  const eyebrow = isLoading
    ? "Loading"
    : isError
      ? "Error loading"
      : connections.length === 0
        ? "No connections yet"
        : visible.length === connections.length
          ? `${connections.length} ${connections.length === 1 ? "connection" : "connections"}`
          : `Showing ${visible.length} of ${connections.length}`;

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        eyebrow={eyebrow}
        title="Connections"
        description="Banks and CSV imports that feed transactions into Breadbox. Re-authenticate broken ones and sync on demand."
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
        <ConnectionsSummary connections={connections} />
      )}

      {attentionCount > 0 && (
        <Alert variant="default" className="border-amber-500/30 bg-amber-500/5">
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
        <FamilyTabs
          users={usersQuery.data ?? []}
          value={userFilter}
          onChange={setUserFilter}
          counts={countsByUserShortId}
          totalCount={connections.length}
        />
      )}

      {isLoading ? (
        <ListCard
          rows={[0, 1, 2]}
          getRowKey={(i) => i}
          renderRow={(i) => (
            <div className="flex items-center gap-4 px-5 py-3.5" key={i}>
              <Skeleton className="size-9 rounded-md" />
              <div className="flex-1 space-y-1.5">
                <Skeleton className="h-3.5 w-40" />
                <Skeleton className="h-3 w-56" />
              </div>
              <div className="space-y-1.5 text-right">
                <Skeleton className="ml-auto h-3.5 w-20" />
                <Skeleton className="ml-auto h-3 w-12" />
              </div>
            </div>
          )}
        />
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
      ) : (
        <ListCard
          rows={visible}
          getRowKey={(c) => c.id}
          renderRow={(c) => (
            <ConnectionRow
              connection={c}
              // /api/v1/accounts exposes connection_id as the parent's
              // short_id (the consistent compact-ID convention), not its UUID.
              stats={accountStats.get(c.short_id)}
              onReauth={openReauthSheet}
              selected={selectedIds.has(c.id)}
              onSelectChange={(next) => toggleRowSelection(c.id, next)}
            />
          )}
          toolbar={
            visible.length === 0 ? undefined : (
            <div className="flex items-center gap-3">
              <Checkbox
                checked={
                  allVisibleSelected
                    ? true
                    : someVisibleSelected
                      ? "indeterminate"
                      : false
                }
                onCheckedChange={() => toggleSelectAll()}
                aria-label={
                  selectedIds.size > 0
                    ? "Clear selection"
                    : "Select all visible"
                }
              />
              <button
                type="button"
                onClick={toggleSelectAll}
                className="hover:text-foreground transition-colors"
              >
                {selectedIds.size > 0
                  ? `${selectedConnections.length} selected`
                  : `Select all visible (${visible.length})`}
              </button>
            </div>
            )
          }
          empty={
            <EmptyState
              title="No connections for this filter"
              description="Switch family member or clear the filter to see other connections."
              className="py-10"
            />
          }
        />
      )}

      {selectedConnections.length > 0 && (
        <SelectionActionBar
          selected={selectedConnections}
          onClear={clearSelection}
        />
      )}

      <ConnectBankSheet
        open={search.action === "connect"}
        onOpenChange={(open) => {
          if (!open) closeSheets();
        }}
      />
      <ReauthSheet
        open={!!search.reauth}
        onOpenChange={(open) => {
          if (!open) closeSheets();
        }}
        connectionShortId={search.reauth}
      />
    </div>
  );
}
