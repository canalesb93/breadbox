import { useState } from "react";
import { Link } from "@tanstack/react-router";
import {
  Building2,
  FileSpreadsheet,
  Landmark,
  Loader2,
  MoreHorizontal,
  Pause,
  Play,
  Plug,
  RefreshCw,
  Unplug,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { withMutationToast } from "@/lib/mutation-toast";
import {
  useDisconnectConnection,
  usePauseConnection,
  useSyncConnection,
} from "@/api/queries/connections";
import type { Connection } from "@/api/types";
import { ConnectionStatusBadge } from "./connection-status-badge";
import {
  formatCurrency,
  primaryBalance,
  providerLabel,
  relativeTime,
  type ConnectionAccountStats,
} from "./connection-utils";

interface ConnectionRowProps {
  connection: Connection;
  stats: ConnectionAccountStats | undefined;
  // Optional callbacks the list passes in to open the (placeholder, in PR-01)
  // re-auth flow. Lifted here so the same trigger can fire from the row banner
  // and the ⋯ menu.
  onReauth: (connection: Connection) => void;
  // Account-level pause flag. Set to true on the connection row briefly while
  // the optimistic mutation is in flight to avoid double-clicks.
  paused?: boolean;
}

const PROVIDER_ICON: Record<string, typeof Building2> = {
  plaid: Building2,
  teller: Landmark,
  csv: FileSpreadsheet,
};

export function ConnectionRow({
  connection,
  stats,
  onReauth,
}: ConnectionRowProps) {
  const sync = useSyncConnection();
  const pause = usePauseConnection();
  const disconnect = useDisconnectConnection();
  const [confirmingDisconnect, setConfirmingDisconnect] = useState(false);

  const Icon = PROVIDER_ICON[connection.provider] ?? Plug;
  const balance = primaryBalance(stats);
  const accountCount = stats?.count ?? 0;
  const canSync =
    connection.provider !== "csv" && connection.status === "active";
  const showReauthBanner =
    connection.status === "pending_reauth" || connection.status === "error";

  async function onSync() {
    await withMutationToast(() => sync.mutateAsync(connection.id), {
      success: `Sync queued for ${connection.institution_name ?? "connection"}.`,
    });
  }

  async function onTogglePause() {
    // Server is the source of truth — we just toggle the inverse of whatever
    // the most recent mutation thinks. The actual paused state lives in the
    // detail response; the list can't tell, so this is best-effort UX.
    await withMutationToast(
      () => pause.mutateAsync({ id: connection.id, paused: true }),
      { success: "Connection paused." },
    );
  }

  async function onDisconnect() {
    const ok = await withMutationToast(
      () => disconnect.mutateAsync(connection.id),
      { success: `Disconnected ${connection.institution_name ?? "connection"}.` },
    );
    if (ok) setConfirmingDisconnect(false);
  }

  return (
    <div className="bg-card overflow-hidden rounded-lg border">
      <div className="flex items-center gap-3 px-4 py-3 sm:gap-4 sm:px-5 sm:py-4">
        <Link
          to="/connections"
          className="flex min-w-0 flex-1 items-center gap-3 sm:gap-4"
          aria-label={`Open ${connection.institution_name ?? "connection"} detail`}
        >
          <div className="bg-muted hidden size-10 shrink-0 items-center justify-center rounded-lg sm:flex">
            <Icon className="text-muted-foreground size-5" />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="truncate text-sm font-semibold sm:text-base">
                {connection.institution_name ?? "Untitled connection"}
              </h3>
              <ConnectionStatusBadge status={connection.status} />
            </div>
            <div className="text-muted-foreground mt-0.5 flex min-w-0 items-center gap-1.5 overflow-hidden text-xs">
              <span className="min-w-0 shrink truncate">
                {connection.user_name ?? "Unassigned"}
              </span>
              <span className="text-muted-foreground/40 shrink-0">·</span>
              <span className="shrink-0 whitespace-nowrap">
                {providerLabel(connection.provider)}
              </span>
              <span className="text-muted-foreground/40 shrink-0">·</span>
              <span className="shrink-0 whitespace-nowrap">
                {connection.last_synced_at
                  ? `synced ${relativeTime(connection.last_synced_at)}`
                  : "never synced"}
              </span>
            </div>
          </div>
        </Link>

        <div className="flex shrink-0 items-center gap-1.5 sm:gap-3">
          {canSync && (
            <Button
              variant="ghost"
              size="icon"
              className="size-8 rounded-full"
              onClick={onSync}
              disabled={sync.isPending}
              aria-label={`Sync ${connection.institution_name ?? "connection"} now`}
              title="Sync now"
            >
              {sync.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <RefreshCw className="size-4" />
              )}
            </Button>
          )}

          <div className="text-right">
            {balance ? (
              <div className="text-sm font-semibold tabular-nums sm:text-base">
                {formatCurrency(balance.amount, balance.currency)}
              </div>
            ) : null}
            <div className="text-muted-foreground text-xs">
              {accountCount} {accountCount === 1 ? "account" : "accounts"}
            </div>
          </div>

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
              {canSync && (
                <DropdownMenuItem onClick={onSync} disabled={sync.isPending}>
                  <RefreshCw className="size-4" /> Sync now
                </DropdownMenuItem>
              )}
              {showReauthBanner && (
                <DropdownMenuItem onClick={() => onReauth(connection)}>
                  <Plug className="size-4" /> Re-authenticate
                </DropdownMenuItem>
              )}
              {connection.status === "active" && (
                <DropdownMenuItem
                  onClick={onTogglePause}
                  disabled={pause.isPending}
                >
                  <Pause className="size-4" /> Pause syncing
                </DropdownMenuItem>
              )}
              {connection.status !== "active" && connection.status !== "disconnected" && (
                <DropdownMenuItem
                  onClick={() =>
                    pause.mutate({ id: connection.id, paused: false })
                  }
                  disabled={pause.isPending}
                >
                  <Play className="size-4" /> Resume syncing
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

      {showReauthBanner && (
        <div className="border-t bg-amber-500/5 px-4 py-2.5 sm:px-5">
          <div className="flex items-center justify-between gap-3">
            <div className="text-amber-700 dark:text-amber-400 text-xs">
              {connection.status === "pending_reauth"
                ? "Login expired. Reconnect to resume syncing."
                : connection.error_message ??
                  "This connection had an error. Reconnect to retry."}
            </div>
            <Button
              size="sm"
              variant="outline"
              className="shrink-0"
              onClick={() => onReauth(connection)}
            >
              Re-authenticate
            </Button>
          </div>
        </div>
      )}

      {confirmingDisconnect && (
        <div className="border-t bg-destructive/5 px-4 py-2.5 sm:px-5">
          <div className="flex items-center justify-between gap-3">
            <div className="text-destructive text-xs">
              Disconnecting wipes credentials and soft-deletes related
              transactions. This can't be undone.
            </div>
            <div className="flex shrink-0 gap-2">
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
          </div>
        </div>
      )}
    </div>
  );
}
