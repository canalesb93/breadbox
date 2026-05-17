import { useMemo, useState } from "react";
import {
  Link,
  useNavigate,
  useParams,
  useSearch,
} from "@tanstack/react-router";
import {
  Activity,
  AlertTriangle,
  Building2,
  ExternalLink,
  FileSpreadsheet,
  Landmark,
  Loader2,
  Pause,
  Play,
  Plug,
  RefreshCw,
  Unplug,
  Upload,
  Wallet,
} from "lucide-react";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/empty-state";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { StatusPanel } from "@/components/status-panel";
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ActionPill } from "@/components/action-pill";
import { RowActionsMenu } from "@/components/row-actions-menu";
import { ColorRailCard } from "@/components/color-rail-card";
import { DetailPageSkeleton } from "@/components/detail-page-skeleton";
import {
  DetailList,
  compactDetailRows,
  type DetailRowData,
} from "@/components/detail-list";
import { Eyebrow } from "@/components/eyebrow";
import { JumpToPill, JumpToRow } from "@/components/jump-to-pill";
import { MetaBadge } from "@/components/meta-badge";
import { SectionCard } from "@/components/section-card";
import { SoftBackButton } from "@/components/soft-back-button";
import { ViewAllPill } from "@/components/view-all-pill";
import { formatLongDate } from "@/lib/format";
import { withMutationToast } from "@/lib/mutation-toast";
import { cn } from "@/lib/utils";
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
import {
  providerLabel,
  relativeTime,
  statusLabel,
  statusTone,
} from "@/features/connections/connection-utils";

// Search-param schema for the detail page. Mirrors the list schema's `reauth`
// so the same Re-auth Sheet can open from either surface. `import_csv` opens
// the CSV upload Sheet inline, pre-targeted to append rows to this connection.
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

// Map the connection status enum onto a CSS colour for the hero rail. Same
// principle as the iter-5/6/7 detail-page heroes — the rail's tint encodes
// *meaning* (the connection's health), not decoration. `disconnected` and
// paused-active drop to muted so the card reads "shelved" instead of
// "active and demanding attention".
function statusRailColor(status: string, paused: boolean): string {
  if (status === "disconnected") return "var(--muted)";
  if (status === "error") return "var(--destructive)";
  if (status === "pending_reauth")
    return "var(--warning, oklch(0.78 0.16 75))";
  if (paused) return "var(--muted)";
  // active
  return "var(--success, oklch(0.65 0.18 145))";
}

export function ConnectionDetailPage() {
  const { id } = useParams({ strict: false }) as { id?: string };
  const search = useSearch({ strict: false }) as ConnectionDetailSearch;
  const navigate = useNavigate();

  const connQuery = useConnection(id);
  const accountsQuery = useAccounts();
  // The /sync/logs endpoint accepts UUIDs only — wait for the connection so
  // we don't fire a global-feed fetch we'll immediately discard.
  const syncLogsQuery = useSyncLogs({
    connectionId: connQuery.data?.id,
    enabled: !!connQuery.data?.id,
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
      <SoftBackButton to="/connections">Back to connections</SoftBackButton>

      {connQuery.isLoading ? (
        <DetailSkeleton />
      ) : connQuery.isError || !connQuery.data ? (
        <EmptyState
          icon={Plug}
          title="Connection not found"
          description="This connection may have been removed, or the link is out of date. Head back to the connections list to pick another."
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

  const canSync = conn.provider !== "csv" && conn.status === "active";
  const isReauthBanner =
    conn.status === "pending_reauth" || conn.status === "error";
  const showFailureBanner =
    conn.consecutive_failures >= 3 && !isReauthBanner;

  // Health metrics — computed from the recent sync logs cache.
  const successRate = useMemo(() => {
    const total = allLogsForActivity.length;
    if (total === 0) return null;
    const success = allLogsForActivity.filter((l) => l.status === "success")
      .length;
    return { success, total, pct: Math.round((success / total) * 100) };
  }, [allLogsForActivity]);

  const failures30 = useMemo(
    () => allLogsForActivity.filter((l) => l.status === "error").length,
    [allLogsForActivity],
  );

  const intervalValue =
    conn.sync_interval_override_minutes == null
      ? "default"
      : String(conn.sync_interval_override_minutes);

  async function onSync() {
    await withMutationToast(() => sync.mutateAsync(conn.id), {
      success: `Sync queued for ${conn.institution_name ?? "connection"}.`,
      successDescription: "New transactions land within a minute.",
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
      <Hero
        conn={conn}
        successRate={successRate}
        canSync={canSync}
        syncPending={sync.isPending}
        pausePending={pause.isPending}
        onSync={onSync}
        onImportCsv={onImportCsv}
        onReauth={onReauth}
        onTogglePause={onTogglePause}
        onDisconnect={() => setConfirmingDisconnect(true)}
      />

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

      {/* Banners — promoted onto `<StatusPanel>` so they speak the same
          tone-tinted vocabulary as Setup, Providers, Home attention-panel,
          and account-detail's connection-status banners. The previous
          open-coded `<Alert>` row used absolute-positioned icons +
          inline amber utilities, which forced a different rhythm from the
          rest of the v2 surfaces and wrapped the Re-authenticate CTA below
          the body text in a half-aligned column on narrow viewports. The
          StatusPanel's `flex items-start gap-3 + trailing` slot keeps the
          icon tile + heading + CTA aligned on mobile, tablet, and desktop. */}
      {isReauthBanner && (
        <StatusPanel
          tone={conn.status === "error" ? "destructive" : "warning"}
          icon={AlertTriangle}
          heading={
            conn.status === "pending_reauth"
              ? "Login expired"
              : "This connection had an error"
          }
          body={
            conn.error_message ??
            "Reconnect to the bank to resume syncing this connection."
          }
          trailing={
            <ActionPill variant="outline" onClick={onReauth}>
              <RefreshCw className="size-3.5" />
              Re-authenticate
            </ActionPill>
          }
        />
      )}

      {showFailureBanner && (
        <StatusPanel
          tone="warning"
          icon={AlertTriangle}
          heading={`${conn.consecutive_failures} syncs failed in a row`}
          body="Recent syncs have been failing. Check the history below for details."
        />
      )}

      <QuickActions conn={conn} />

      {/* Two-column body: primary content on the left, settings + details
          on the right. Mirrors the Account-detail layout (which inverted
          TX-detail's split for the same reason — when a page has more
          first-class affordances on the side it earns the sidebar slot).
          On <lg we stack: the sidebar drops below the primary column.  */}
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <div className="min-w-0 space-y-6">
          {/* Sync activity */}
          <SectionCard
            title="Sync activity"
            icon={<Activity className="text-muted-foreground size-4" />}
            action={<Eyebrow>Last 7 days</Eyebrow>}
          >
            {syncLogsLoading ? (
              <Skeleton className="h-[72px] w-full" />
            ) : (
              <SyncActivityBars logs={allLogsForActivity} days={7} />
            )}
          </SectionCard>

          {/* Accounts */}
          <SectionCard
            title={`Accounts (${conn.account_count})`}
            icon={<Wallet className="text-muted-foreground size-4" />}
            action={
              <Button
                variant="ghost"
                size="sm"
                asChild
                className="h-7 gap-1 px-2 text-xs"
              >
                <Link to="/accounts">
                  Open in Accounts
                  <ExternalLink className="size-3" />
                </Link>
              </Button>
            }
          >
            {accountsQueryLoading(accounts.length, conn.account_count) ? (
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {Array.from({
                  length: Math.min(conn.account_count || 1, 3),
                }).map((_, i) => (
                  <Skeleton key={i} className="h-[68px] w-full rounded-lg" />
                ))}
              </div>
            ) : (
              <ConnectionAccountsList accounts={accounts} />
            )}
          </SectionCard>

          {/* Sync history */}
          <SectionCard
            title="Sync history"
            icon={<RefreshCw className="text-muted-foreground size-4" />}
            action={<Eyebrow>Last 10</Eyebrow>}
            footer={
              syncLogs.length > 0 ? (
                <ViewAllPill to="/sync-logs" align="footer">
                  View all
                </ViewAllPill>
              ) : undefined
            }
          >
            {syncLogsLoading ? (
              <div className="space-y-2">
                {Array.from({ length: 4 }).map((_, i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </div>
            ) : (
              <SyncHistoryList logs={syncLogs} />
            )}
          </SectionCard>
        </div>

        <aside className="space-y-6">
          <SettingsCard
            conn={conn}
            intervalValue={intervalValue}
            onIntervalChange={onIntervalChange}
            setIntervalPending={setInterval.isPending}
            pausePending={pause.isPending}
            onTogglePause={onTogglePause}
          />
          <DetailsCard
            conn={conn}
            successRate={successRate}
            failures30={failures30}
            totalSyncs={allLogsForActivity.length}
            syncLogsLoading={syncLogsLoading}
          />
        </aside>
      </div>
    </div>
  );
}

// Hero condenses identity + status + the single most useful metric (last
// sync) into one composed card. The 4px left rail is tinted by the
// connection's status (success / warning / destructive / muted) — the
// colour encodes meaning, not decoration. Same vocabulary as the iter-5/6/7
// TX / Account / Category detail heroes, paired with a small uppercase
// "Connection" eyebrow so colour never carries the signal alone.
function Hero({
  conn,
  successRate,
  canSync,
  syncPending,
  pausePending,
  onSync,
  onImportCsv,
  onReauth,
  onTogglePause,
  onDisconnect,
}: {
  conn: ConnectionDetail;
  successRate: { success: number; total: number; pct: number } | null;
  canSync: boolean;
  syncPending: boolean;
  pausePending: boolean;
  onSync: () => void;
  onImportCsv: () => void;
  onReauth: () => void;
  onTogglePause: () => void;
  onDisconnect: () => void;
}) {
  const Icon = PROVIDER_ICON[conn.provider] ?? Plug;
  const accent = statusRailColor(conn.status, conn.paused);
  const tone = statusTone(conn.status);

  return (
    <ColorRailCard
      accent={accent}
      footer={
        <>
          {canSync && (
            <ActionPill onClick={onSync} disabled={syncPending}>
              {syncPending ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <RefreshCw className="size-3.5" />
              )}
              Sync now
            </ActionPill>
          )}
          {conn.provider === "csv" && conn.status !== "disconnected" && (
            <ActionPill onClick={onImportCsv}>
              <Upload className="size-3.5" />
              Import more
            </ActionPill>
          )}
          {conn.status !== "disconnected" && (
            <ActionPill onClick={onReauth}>
              <Plug className="size-3.5" />
              Re-authenticate
            </ActionPill>
          )}
          <RowActionsMenu
            label="Connection actions"
            size="xs"
            triggerClassName="rounded-full"
          >
            {conn.status !== "disconnected" && (
              <DropdownMenuItem
                onClick={onTogglePause}
                disabled={pausePending}
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
            <DropdownMenuItem variant="destructive" onClick={onDisconnect}>
              <Unplug className="size-4" /> Disconnect…
            </DropdownMenuItem>
          </RowActionsMenu>
        </>
      }
    >
      <div className="grid gap-5 px-5 py-5 sm:gap-6 sm:px-7 sm:py-6 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start lg:gap-10">
        {/* Identity column */}
        <div className="min-w-0 space-y-3">
          <div className="flex items-start gap-4">
            <div
              className={cn(
                "bg-muted flex size-12 shrink-0 items-center justify-center rounded-lg",
                (conn.status === "disconnected" || conn.paused) && "opacity-60",
              )}
            >
              <Icon className="text-muted-foreground size-5" />
            </div>
            <div className="min-w-0 space-y-1">
              <Eyebrow as="p" variant="hero">
                Connection
              </Eyebrow>
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="truncate text-xl font-semibold tracking-tight">
                  {conn.institution_name ?? "Untitled connection"}
                </h1>
                <ConnectionStatusBadge status={conn.status} />
                {conn.paused && (
                  <MetaBadge icon={Pause} variant="secondary">
                    Paused
                  </MetaBadge>
                )}
              </div>
              <p className="text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs">
                <span>{providerLabel(conn.provider)}</span>
                {conn.user_name && (
                  <>
                    <span aria-hidden className="opacity-50">·</span>
                    <span>{conn.user_name}</span>
                  </>
                )}
                <span aria-hidden className="opacity-50">·</span>
                <span>
                  {conn.account_count}{" "}
                  {conn.account_count === 1 ? "account" : "accounts"}
                </span>
              </p>
            </div>
          </div>
        </div>

        {/* Metric column — last sync as the headline number, success rate
            as a smaller anchor underneath. Mirrors the Account-detail
            balance-pill + amount + secondary-line vertical rhythm. */}
        <div className="flex flex-col items-start gap-1.5 lg:items-end lg:text-right">
          <div
            className={cn(
              "inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[10px] font-medium tracking-wide uppercase whitespace-nowrap",
              tone === "active" &&
                "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
              tone === "warning" &&
                "bg-amber-500/10 text-amber-700 dark:text-amber-400",
              tone === "destructive" && "bg-destructive/10 text-destructive",
              tone === "muted" && "bg-muted text-muted-foreground",
            )}
          >
            {conn.last_synced_at ? "Last synced" : statusLabel(conn.status)}
          </div>
          <div
            className={cn(
              "font-semibold tabular-nums",
              "text-2xl sm:text-3xl",
              (conn.status === "disconnected" || conn.paused) && "opacity-60",
            )}
          >
            {conn.last_synced_at ? relativeTime(conn.last_synced_at) : "Never"}
          </div>
          {successRate ? (
            <p className="text-muted-foreground pt-1 text-[11px] tabular-nums">
              {successRate.pct}% success ·{" "}
              {successRate.success}/{successRate.total} recent
            </p>
          ) : conn.consecutive_failures > 0 ? (
            <p className="text-destructive/80 pt-1 text-[11px]">
              {conn.consecutive_failures} consecutive failures
            </p>
          ) : null}
        </div>
      </div>
    </ColorRailCard>
  );
}

// QuickActions matches the iter-5/6 "Jump to" pill row used on TX and
// Account detail — labelled lateral navigation that surfaces concrete
// targets (the connection's accounts, the global sync-logs feed) rather
// than just verbs.
function QuickActions(_props: { conn: ConnectionDetail }) {
  return (
    <JumpToRow>
      <JumpToPill asChild>
        <Link to="/accounts">
          <Wallet className="size-3" />
          All accounts
        </Link>
      </JumpToPill>
      <JumpToPill asChild>
        <Link to="/sync-logs">
          <Activity className="size-3" />
          Sync log
        </Link>
      </JumpToPill>
    </JumpToRow>
  );
}

function SettingsCard({
  conn,
  intervalValue,
  onIntervalChange,
  setIntervalPending,
  pausePending,
  onTogglePause,
}: {
  conn: ConnectionDetail;
  intervalValue: string;
  onIntervalChange: (value: string) => void;
  setIntervalPending: boolean;
  pausePending: boolean;
  onTogglePause: () => void;
}) {
  return (
    <SectionCard title="Settings" bodyClassName="space-y-4 px-5 py-5 text-sm">
      <div className="space-y-1.5">
        <label
          htmlFor="sync-interval"
          className="text-muted-foreground text-[10px] font-medium tracking-[0.1em] uppercase"
        >
          Sync interval
        </label>
        <Select
          value={intervalValue}
          onValueChange={onIntervalChange}
          disabled={setIntervalPending || conn.status === "disconnected"}
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
          <div className="text-xs font-medium">Schedule</div>
          <div className="text-muted-foreground text-xs">
            {conn.paused ? "Scheduled syncs paused" : "Syncing on schedule"}
          </div>
        </div>
        <ActionPill
          variant="outline"
          onClick={onTogglePause}
          disabled={pausePending || conn.status === "disconnected"}
        >
          {pausePending ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : conn.paused ? (
            <Play className="size-3.5" />
          ) : (
            <Pause className="size-3.5" />
          )}
          {conn.paused ? "Resume" : "Pause"}
        </ActionPill>
      </div>
    </SectionCard>
  );
}

function DetailsCard({
  conn,
  successRate,
  failures30,
  totalSyncs,
  syncLogsLoading,
}: {
  conn: ConnectionDetail;
  successRate: { success: number; total: number; pct: number } | null;
  failures30: number;
  totalSyncs: number;
  syncLogsLoading: boolean;
}) {
  const healthRows: DetailRowData[] = compactDetailRows([
    {
      label: "Success rate",
      value: successRate
        ? `${successRate.pct}%`
        : syncLogsLoading
          ? "…"
          : "—",
    },
    {
      label: "Failures",
      value: syncLogsLoading ? "…" : String(failures30),
    },
    {
      label: "Total syncs",
      value: syncLogsLoading ? "…" : String(totalSyncs),
    },
  ]);

  const providerRows: DetailRowData[] = compactDetailRows([
    { label: "Provider", value: providerLabel(conn.provider) },
    conn.user_name ? { label: "User", value: conn.user_name } : null,
    conn.institution_id
      ? { label: "Institution", value: conn.institution_id, mono: true }
      : null,
  ]);

  const referenceRows: DetailRowData[] = compactDetailRows([
    { label: "ID", value: conn.short_id, mono: true },
    { label: "Created", value: formatLongDate(conn.created_at.slice(0, 10)) },
    {
      label: "Updated",
      value: formatLongDate(conn.updated_at.slice(0, 10)),
    },
  ]);

  return (
    <SectionCard title="Details" bodyClassName="space-y-5 px-5 py-5 text-sm">
      <DetailList label="Health" rows={healthRows} />
      <DetailList label="Provider" rows={providerRows} />
      <DetailList label="Reference" rows={referenceRows} />
    </SectionCard>
  );
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
  // Hero matches the loaded `<ColorRailCard>` shape: status-tinted rail +
  // `rounded-lg` icon tile + bordered action-strip footer (Sync now /
  // Re-authenticate / overflow). The `DetailPageSkeleton` primitive
  // converges every v2 detail page on a single skeleton shape — no
  // jump-pill strip here because connection-detail puts its actions in
  // the hero footer instead.
  return (
    <DetailPageSkeleton
      hero={{ tileShape: "rounded-lg", withFooter: true }}
      main={["h-32", "h-48", "h-48"]}
      sidebar={["h-40", "h-48"]}
    />
  );
}
