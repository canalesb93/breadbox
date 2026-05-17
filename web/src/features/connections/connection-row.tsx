import { useState } from "react";
import { Link } from "@tanstack/react-router";
import {
  Building2,
  FileSpreadsheet,
  Landmark,
  Loader2,
  Pause,
  Plug,
  Unplug,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import { RowActionsMenu } from "@/components/row-actions-menu";
import { formatBalance } from "@/lib/format";
import { withMutationToast } from "@/lib/mutation-toast";
import { cn } from "@/lib/utils";
import {
  useDisconnectConnection,
  usePauseConnection,
} from "@/api/queries/connections";
import type { Connection } from "@/api/types";
import { ConnectionStatusBadge } from "./connection-status-badge";
import {
  primaryBalance,
  providerLabel,
  relativeTime,
  type ConnectionAccountStats,
} from "./connection-utils";

interface ConnectionRowProps {
  connection: Connection;
  stats: ConnectionAccountStats | undefined;
  // Lifted to the page so the same trigger fires from the inline banner and
  // the row's ⋯ menu.
  onReauth: (connection: Connection) => void;
  // Multi-select state, lifted to the page so the bulk action bar can read
  // the union of selected ids. Optional — pages that don't render a bulk bar
  // can omit and the checkbox UI stays hidden.
  selected?: boolean;
  onSelectChange?: (next: boolean) => void;
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
  selected,
  onSelectChange,
}: ConnectionRowProps) {
  const pause = usePauseConnection();
  const disconnect = useDisconnectConnection();
  const [confirmingDisconnect, setConfirmingDisconnect] = useState(false);

  const Icon = PROVIDER_ICON[connection.provider] ?? Plug;
  const balance = primaryBalance(stats);
  const accountCount = stats?.count ?? 0;
  const showReauthBanner =
    connection.status === "pending_reauth" || connection.status === "error";

  async function onTogglePause() {
    // The list endpoint doesn't return the paused flag, so this row can only
    // pause (not toggle). Resume lives on the detail page where paused is
    // visible, and on the bulk action bar.
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
    <div
      className={cn(
        "group transition-colors",
        selected && "bg-primary/5",
      )}
    >
      <div
        className={cn(
          "hover:bg-muted/40 flex items-center gap-3 px-5 py-3.5 transition-colors sm:gap-4",
          selected && "hover:bg-primary/5",
        )}
      >
        {onSelectChange && (
          // Wrapper stops the click from bubbling to the surrounding Link, so
          // toggling selection never navigates to the detail page. The label
          // is generous (full size) so it's an easy mobile tap target.
          <label
            className="flex shrink-0 cursor-pointer items-center justify-center p-1"
            onClick={(e) => e.stopPropagation()}
            aria-label={`Select ${connection.institution_name ?? "connection"}`}
          >
            <Checkbox
              checked={!!selected}
              onCheckedChange={(value) => onSelectChange(!!value)}
            />
          </label>
        )}
        <Link
          to="/connections/$id"
          params={{ id: connection.short_id }}
          className="flex min-w-0 flex-1 items-center gap-3 sm:gap-4"
          aria-label={`Open ${connection.institution_name ?? "connection"} detail`}
        >
          <div
            className={cn(
              "bg-muted/40 hidden size-9 shrink-0 items-center justify-center rounded-md border sm:flex",
              showReauthBanner && "bg-amber-500/10 border-amber-500/30",
            )}
          >
            <Icon
              className={cn(
                "text-muted-foreground size-4",
                showReauthBanner && "text-amber-700 dark:text-amber-400",
              )}
            />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="truncate text-sm font-semibold">
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
          <div className="text-right">
            {balance ? (
              <div className="text-sm font-semibold tabular-nums">
                {formatBalance(balance.amount, balance.currency)}
              </div>
            ) : null}
            <div className="text-muted-foreground text-xs">
              {accountCount} {accountCount === 1 ? "account" : "accounts"}
            </div>
          </div>

          <RowActionsMenu label="Connection actions">
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
            <DropdownMenuSeparator />
            <DropdownMenuItem
              variant="destructive"
              onClick={() => setConfirmingDisconnect(true)}
            >
              <Unplug className="size-4" /> Disconnect…
            </DropdownMenuItem>
          </RowActionsMenu>
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
