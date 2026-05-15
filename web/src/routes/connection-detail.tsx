import { useMemo, useState } from "react";
import {
  Link,
  useNavigate,
  useParams,
  useSearch,
} from "@tanstack/react-router";
import {
  ArrowLeft,
  Building2,
  ExternalLink,
  FileSpreadsheet,
  Landmark,
  Loader2,
  MoreHorizontal,
  Pause,
  Play,
  Plug,
  RefreshCw,
  Unplug,
  AlertTriangle,
} from "lucide-react";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/empty-state";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { withMutationToast } from "@/lib/mutation-toast";
import {
  useConnection,
  useDisconnectConnection,
  usePauseConnection,
  useSetSyncInterval,
  useSyncConnection,
} from "@/api/queries/connections";
import { useAccounts } from "@/api/queries/accounts";
import { useSyncLogs } from "@/api/queries/sync-logs";
import type { Account, ConnectionDetail } from "@/api/types";
import type { SyncLog } from "@/api/queries/sync-logs";
import { ConnectionStatusBadge } from "@/features/connections/connection-status-badge";
import { ConnectBankSheet } from "@/features/connections/connect-bank-sheet";
import { ReauthSheet } from "@/features/connections/reauth-sheet";
import { ConnectionAccountsList } from "@/features/connections/connection-accounts-list";
import { SyncActivityBars } from "@/features/connections/sync-activity-bars";
import { SyncHistoryList } from "@/features/connections/sync-history-list";
import { providerLabel, relativeTime } from "@/features/connections/connection-utils";
import { Upload } from "lucide-react";

// Search-param schema for the detail page. Mirrors the list schema's `reauth`
// so the same coming-soon Sheet can open from either surface. `import_csv`
// opens the CSV upload Sheet inline, pre-targeted to append rows to this
// connection.
export const connectionDetailSearchSchema = z.object({
  reauth: z.string().optional(),
  import_csv: z.string().optional(),
});

type ConnectionDetailSearch = z.infer<typeof connectionDetailSearchSchema>;

// Same option set as the v1 connection_detail.templ — "global default" maps
// to a cleared (null) override; the others are minute counts.
const SYNC_INTERVAL_OPTIONS: { value: string; label: string }[] = [
  { value: "default", label: "Global default" },
  { value: "15", label: "Every 15 min" },
  { value: "30", label: "Every 30 min" },
  { value: "60", label: "Every hour" },
  { value: "120", label: "Every 2 hours" },
  { value: "240", label: "Every 4 hours" },
  { value: "720", label: "Every 12 hours" },
  { value: "1440", label: "Every 24 hours" },
];

const PROVIDER_ICON: Record<string, typeof Building2> = {
  plaid: Building2,
  teller: Landmark,
  csv: FileSpreadsheet,
};

export function ConnectionDetailPage() {
  const { id } = useParams({ strict: false }) as { id?: string };
  const search = useSearch({ strict: false }) as ConnectionDetailSearch;
  const navigate = useNavigate();

  const connQuery = useConnection(id);
  const accountsQuery = useAccounts();
  // The /sync/logs endpoint accepts UUIDs only — we pass the connection's
  // UUID once it's loaded.
  const syncLogsQuery = useSyncLogs({
    connectionId: connQuery.data?.id,
    limit: 30,
  });

  const accountsForConn = useMemo(() => {
    if (!connQuery.data || !accountsQuery.data) return [];
    return accountsQuery.data.filter(
      (a) => a.connection_id === connQuery.data!.short_id,
    );
  }, [accountsQuery.data, connQuery.data]);

  const recentLogs = useMemo(
    () => syncLogsQuery.data?.sync_logs.slice(0, 10) ?? [],
    [syncLogsQuery.data],
  );

  function openReauth() {
    if (!connQuery.data) return;
    navigate({
      to: "/connections/$id",
      params: { id: connQuery.data.short_id },
      search: (prev: ConnectionDetailSearch) => ({
        ...prev,
        reauth: connQuery.data!.short_id,
      }),
      replace: false,
    });
  }

  function closeReauth() {
    if (!connQuery.data) return;
    navigate({
      to: "/connections/$id",
      params: { id: connQuery.data.short_id },
      search: (prev: ConnectionDetailSearch) => ({
        ...prev,
        reauth: undefined,
      }),
      replace: true,
    });
  }

  function openImportCsv() {
    if (!connQuery.data) return;
    navigate({
      to: "/connections/$id",
      params: { id: connQuery.data.short_id },
      search: (prev: ConnectionDetailSearch) => ({
        ...prev,
        import_csv: connQuery.data!.short_id,
      }),
      replace: false,
    });
  }

  function closeImportCsv() {
    if (!connQuery.data) return;
    navigate({
      to: "/connections/$id",
      params: { id: connQuery.data.short_id },
      search: (prev: ConnectionDetailSearch) => ({
        ...prev,
        import_csv: undefined,
      }),
      replace: true,
    });
  }

  return (
    <div className="mx-auto max-w-5xl">
      <Button variant="ghost" size="sm" asChild className="mb-4 -ml-2">
        <Link to="/connections">
          <ArrowLeft className="size-4" />
          Connections
        </Link>
      </Button>

      {connQuery.isLoading ? (
        <DetailSkeleton />
      ) : connQuery.isError || !connQuery.data ? (
        <EmptyState
          icon={Plug}
          title="Connection not found"
          description="It may have been disconnected, or the link is wrong."
          action={
            <Button variant="outline" asChild>
              <Link to="/connections">Back to connections</Link>
            </Button>
          }
        />
      ) : (
        <DetailBody
          conn={connQuery.data}
          accounts={accountsForConn}
          syncLogs={recentLogs}
          allLogsForActivity={syncLogsQuery.data?.sync_logs ?? []}
          syncLogsLoading={syncLogsQuery.isLoading}
          onReauth={openReauth}
          onImportCsv={openImportCsv}
        />
      )}

      <ReauthSheet
        open={!!search.reauth}
        onOpenChange={(open) => {
          if (!open) closeReauth();
        }}
        connectionShortId={search.reauth}
      />
      {connQuery.data?.provider === "csv" && (
        <ConnectBankSheet
          open={!!search.import_csv}
          onOpenChange={(open) => {
            if (!open) closeImportCsv();
          }}
          appendToConnectionId={connQuery.data.short_id}
        />
      )}
    </div>
  );
}

interface DetailBodyProps {
  conn: ConnectionDetail;
  accounts: Account[];
  syncLogs: SyncLog[];
  allLogsForActivity: SyncLog[];
  syncLogsLoading: boolean;
  onReauth: () => void;
  onImportCsv: () => void;
}

function DetailBody({
  conn,
  accounts,
  syncLogs,
  allLogsForActivity,
  syncLogsLoading,
  onReauth,
  onImportCsv,
}: DetailBodyProps) {
  const sync = useSyncConnection();
  const pause = usePauseConnection();
  const disconnect = useDisconnectConnection();
  const setInterval = useSetSyncInterval();
  const [confirmingDisconnect, setConfirmingDisconnect] = useState(false);

  const Icon = PROVIDER_ICON[conn.provider] ?? Plug;
  const canSync = conn.provider !== "csv" && conn.status === "active";
  const isReauthBanner =
    conn.status === "pending_reauth" || conn.status === "error";
  const showFailureBanner =
    conn.consecutive_failures >= 3 && !isReauthBanner;

  // Health card metrics — computed from the recent sync logs cache.
  const successRate = useMemo(() => {
    const total = allLogsForActivity.length;
    if (total === 0) return null;
    const success = allLogsForActivity.filter((l) => l.status === "success").length;
    return { success, total, pct: Math.round((success / total) * 100) };
  }, [allLogsForActivity]);

  const failures30 = useMemo(
    () => allLogsForActivity.filter((l) => l.status === "error").length,
    [allLogsForActivity],
  );

  const intervalValue = conn.sync_interval_override_minutes == null
    ? "default"
    : String(conn.sync_interval_override_minutes);

  async function onSync() {
    await withMutationToast(() => sync.mutateAsync(conn.id), {
      success: `Sync queued for ${conn.institution_name ?? "connection"}.`,
    });
  }

  async function onTogglePause() {
    const next = !conn.paused;
    await withMutationToast(
      () => pause.mutateAsync({ id: conn.id, paused: next }),
      { success: next ? "Connection paused." : "Connection resumed." },
    );
  }

  async function onDisconnect() {
    const ok = await withMutationToast(
      () => disconnect.mutateAsync(conn.id),
      { success: `Disconnected ${conn.institution_name ?? "connection"}.` },
    );
    if (ok) setConfirmingDisconnect(false);
  }

  async function onIntervalChange(value: string) {
    const minutes = value === "default" ? null : Number(value);
    await withMutationToast(
      () => setInterval.mutateAsync({ id: conn.id, minutes }),
      {
        success:
          minutes == null
            ? "Sync interval reset to global default."
            : `Sync interval set to ${formatIntervalLabel(minutes)}.`,
      },
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 items-center gap-3">
          <div className="bg-muted flex size-11 shrink-0 items-center justify-center rounded-lg">
            <Icon className="text-muted-foreground size-5" />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="truncate text-xl font-semibold tracking-tight">
                {conn.institution_name ?? "Untitled connection"}
              </h1>
              <ConnectionStatusBadge status={conn.status} />
              {conn.paused && (
                <span className="bg-muted text-muted-foreground rounded-full px-2 py-0.5 text-xs">
                  Paused
                </span>
              )}
            </div>
            <div className="text-muted-foreground mt-1 flex flex-wrap items-center gap-1.5 text-xs">
              <span>{providerLabel(conn.provider)}</span>
              {conn.user_name && (
                <>
                  <span className="text-muted-foreground/40">·</span>
                  <span>{conn.user_name}</span>
                </>
              )}
              {conn.last_synced_at && (
                <>
                  <span className="text-muted-foreground/40">·</span>
                  <span>Synced {relativeTime(conn.last_synced_at)}</span>
                </>
              )}
            </div>
          </div>
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {canSync && (
            <Button
              variant="outline"
              size="sm"
              onClick={onSync}
              disabled={sync.isPending}
            >
              {sync.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <RefreshCw className="size-4" />
              )}
              Sync now
            </Button>
          )}
          {conn.provider === "csv" && conn.status !== "disconnected" && (
            <Button variant="outline" size="sm" onClick={onImportCsv}>
              <Upload className="size-4" />
              Import more
            </Button>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="size-8 rounded-full"
                aria-label="Connection actions"
              >
                <MoreHorizontal className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {conn.status !== "disconnected" && (
                <DropdownMenuItem onClick={onReauth}>
                  <Plug className="size-4" /> Re-authenticate
                </DropdownMenuItem>
              )}
              {conn.status !== "disconnected" && (
                <DropdownMenuItem
                  onClick={onTogglePause}
                  disabled={pause.isPending}
                >
                  {conn.paused ? (
                    <>
                      <Play className="size-4" /> Resume syncing
                    </>
                  ) : (
                    <>
                      <Pause className="size-4" /> Pause syncing
                    </>
                  )}
                </DropdownMenuItem>
              )}
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant="destructive"
                onClick={() => setConfirmingDisconnect(true)}
              >
                <Unplug className="size-4" /> Disconnect…
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Inline disconnect confirm */}
      {confirmingDisconnect && (
        <Alert variant="destructive">
          <AlertTitle>Disconnect this connection?</AlertTitle>
          <AlertDescription className="space-y-2">
            <p>
              Disconnecting wipes credentials and soft-deletes related
              transactions. This can&apos;t be undone.
            </p>
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setConfirmingDisconnect(false)}
                disabled={disconnect.isPending}
              >
                Cancel
              </Button>
              <Button
                size="sm"
                variant="destructive"
                onClick={onDisconnect}
                disabled={disconnect.isPending}
              >
                {disconnect.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : null}
                Disconnect
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      )}

      {/* Banners */}
      {isReauthBanner && (
        <Alert
          className={
            conn.status === "error"
              ? "border-destructive/30 bg-destructive/5"
              : "border-amber-500/30 bg-amber-500/5"
          }
        >
          <AlertTriangle
            className={
              conn.status === "error"
                ? "text-destructive size-4"
                : "size-4 text-amber-700 dark:text-amber-400"
            }
          />
          <AlertTitle
            className={
              conn.status === "error"
                ? "text-destructive"
                : "text-amber-700 dark:text-amber-400"
            }
          >
            {conn.status === "pending_reauth"
              ? "Login expired"
              : "This connection had an error"}
          </AlertTitle>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
            <span>
              {conn.error_message ??
                "Reconnect to the bank to resume syncing this connection."}
            </span>
            <Button size="sm" variant="outline" onClick={onReauth}>
              Re-authenticate
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {showFailureBanner && (
        <Alert className="border-amber-500/30 bg-amber-500/5">
          <AlertTriangle className="size-4 text-amber-700 dark:text-amber-400" />
          <AlertTitle className="text-amber-700 dark:text-amber-400">
            {conn.consecutive_failures} syncs failed in a row
          </AlertTitle>
          <AlertDescription>
            Recent syncs have been failing. Check the history below for details.
          </AlertDescription>
        </Alert>
      )}

      {/* Health + Settings cards */}
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Health</CardTitle>
          </CardHeader>
          <CardContent className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
            <Stat
              label="Success rate"
              value={
                successRate
                  ? `${successRate.pct}%`
                  : syncLogsLoading
                    ? "…"
                    : "—"
              }
              hint={
                successRate
                  ? `${successRate.success}/${successRate.total} (last 30)`
                  : undefined
              }
            />
            <Stat
              label="Last sync"
              value={
                conn.last_synced_at ? relativeTime(conn.last_synced_at) : "Never"
              }
            />
            <Stat
              label="Failures (recent)"
              value={syncLogsLoading ? "…" : String(failures30)}
              hint="last 30 syncs"
            />
            <Stat
              label="Total syncs"
              value={
                syncLogsQueryTotal(allLogsForActivity.length, syncLogsLoading)
              }
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Settings</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4 text-sm">
            <div className="space-y-1.5">
              <label
                htmlFor="sync-interval"
                className="text-muted-foreground text-xs"
              >
                Sync interval
              </label>
              <Select
                value={intervalValue}
                onValueChange={onIntervalChange}
                disabled={
                  setInterval.isPending || conn.status === "disconnected"
                }
              >
                <SelectTrigger id="sync-interval" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SYNC_INTERVAL_OPTIONS.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-muted-foreground text-xs">
                Override the global default scheduled-sync cadence for this
                connection.
              </p>
            </div>
            <div className="border-border/40 flex items-center justify-between border-t pt-3">
              <div>
                <div className="text-xs font-medium">Status</div>
                <div className="text-muted-foreground text-xs">
                  {conn.paused ? "Scheduled syncs paused" : "Syncing on schedule"}
                </div>
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={onTogglePause}
                disabled={pause.isPending || conn.status === "disconnected"}
              >
                {pause.isPending ? (
                  <Loader2 className="size-3.5 animate-spin" />
                ) : conn.paused ? (
                  <Play className="size-3.5" />
                ) : (
                  <Pause className="size-3.5" />
                )}
                {conn.paused ? "Resume" : "Pause"}
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* 7-day sync activity */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center justify-between text-base">
            <span>Sync activity</span>
            <span className="text-muted-foreground text-xs font-normal">
              7 days
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {syncLogsLoading ? (
            <Skeleton className="h-[72px] w-full" />
          ) : (
            <SyncActivityBars logs={allLogsForActivity} days={7} />
          )}
        </CardContent>
      </Card>

      {/* Accounts */}
      <section className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold">
            Accounts ({conn.account_count})
          </h2>
          <Button variant="ghost" size="sm" asChild>
            <Link to="/accounts">
              Open in Accounts
              <ExternalLink className="size-3.5" />
            </Link>
          </Button>
        </div>
        {accountsQueryLoading(accounts.length, conn.account_count) ? (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: Math.min(conn.account_count || 1, 3) }).map(
              (_, i) => (
                <Skeleton key={i} className="h-[68px] w-full rounded-lg" />
              ),
            )}
          </div>
        ) : (
          <ConnectionAccountsList accounts={accounts} />
        )}
      </section>

      {/* Sync history */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center justify-between text-base">
            <span>Sync history</span>
            <span className="text-muted-foreground text-xs font-normal">
              Last 10
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {syncLogsLoading ? (
            <div className="space-y-2">
              {Array.from({ length: 4 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : (
            <SyncHistoryList logs={syncLogs} />
          )}
        </CardContent>
        {syncLogs.length > 0 && (
          <div className="border-border/40 border-t px-6 py-3 text-right text-xs">
            <Link
              to="/sync-logs"
              className="text-muted-foreground hover:text-foreground"
            >
              View all →
            </Link>
          </div>
        )}
      </Card>
    </div>
  );
}

function Stat({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint?: string;
}) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-base font-semibold tabular-nums">{value}</dd>
      {hint && <dd className="text-muted-foreground text-[0.65rem]">{hint}</dd>}
    </div>
  );
}

function syncLogsQueryTotal(loadedCount: number, loading: boolean): string {
  if (loading) return "…";
  // The `/sync/logs` page response includes a global `total` we could surface
  // via the query; the simpler "loaded" approximation is fine for the recent
  // window we render. A full count requires a separate /sync/stats query.
  return String(loadedCount);
}

function accountsQueryLoading(loaded: number, expected: number): boolean {
  // The accounts list is a global cached fetch; treat the section as loading
  // only if we have nothing yet AND the connection claims accounts exist.
  return loaded === 0 && expected > 0;
}

function formatIntervalLabel(minutes: number): string {
  if (minutes < 60) return `${minutes} min`;
  const hours = minutes / 60;
  if (hours < 24) return `${hours}h`;
  return `${hours / 24}d`;
}

function DetailSkeleton() {
  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Skeleton className="size-11 rounded-lg" />
        <div className="space-y-2">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-4 w-56" />
        </div>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <Skeleton className="h-40 rounded-xl" />
        <Skeleton className="h-40 rounded-xl" />
      </div>
      <Skeleton className="h-32 rounded-xl" />
      <Skeleton className="h-48 rounded-xl" />
    </div>
  );
}
