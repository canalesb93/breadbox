import { useState } from "react";
import { Pause, Play, RefreshCw, Unplug, X } from "lucide-react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { KbdTooltip } from "@/components/kbd-tooltip";
import { withMutationToast } from "@/lib/mutation-toast";
import {
  useDisconnectConnection,
  usePauseConnection,
  useSyncConnection,
} from "@/api/queries/connections";
import type { Connection } from "@/api/types";

// Cap concurrent per-connection mutations. The backend handlers are fine with
// fan-out, but the household connections universe is small and a thundering
// herd of 50 concurrent DELETEs is impolite. Sequential per chunk smooths it.
const BATCH_LIMIT = 25;

function chunk<T>(items: T[], size: number): T[][] {
  const out: T[][] = [];
  for (let i = 0; i < items.length; i += size) {
    out.push(items.slice(i, i + size));
  }
  return out;
}

interface SelectionActionBarProps {
  selected: Connection[];
  onClear: () => void;
}

// SelectionActionBar is the floating bar shown when ≥1 connection row is
// checked. Mirrors the transactions page layout (sticky-bottom pill, count +
// actions + clear). Each button fans the selection out into the existing
// per-id mutations — there's no batch endpoint for connections, but the
// universe is small enough that chunked sequential calls are fine.
export function SelectionActionBar({
  selected,
  onClear,
}: SelectionActionBarProps) {
  const sync = useSyncConnection();
  const pause = usePauseConnection();
  const disconnect = useDisconnectConnection();
  const [confirmOpen, setConfirmOpen] = useState(false);

  const isPending = sync.isPending || pause.isPending || disconnect.isPending;

  const syncable = selected.filter(
    (c) => c.provider !== "csv" && c.status === "active",
  );
  const pausable = selected.filter((c) => c.status === "active");
  // Resume = un-pause anything that isn't disconnected (covers paused-active
  // rows the list can't see, plus pending_reauth/error rows that may have been
  // paused alongside their re-auth state).
  const resumable = selected.filter(
    (c) => c.status !== "disconnected" && c.status !== "active",
  );

  async function runChunked<T>(
    items: T[],
    fn: (item: T) => Promise<unknown>,
  ): Promise<void> {
    for (const batch of chunk(items, BATCH_LIMIT)) {
      // Promise.all within a chunk = bounded concurrency; await between chunks
      // smooths out the request rate.
      await Promise.all(batch.map(fn));
    }
  }

  async function onSync() {
    if (syncable.length === 0) return;
    const ok = await withMutationToast(
      () => runChunked(syncable, (c) => sync.mutateAsync(c.id)),
      { success: `Sync queued for ${syncable.length} connection${syncable.length === 1 ? "" : "s"}.` },
    );
    if (ok) onClear();
  }

  async function onPause() {
    if (pausable.length === 0) return;
    const ok = await withMutationToast(
      () =>
        runChunked(pausable, (c) =>
          pause.mutateAsync({ id: c.id, paused: true }),
        ),
      { success: `Paused ${pausable.length} connection${pausable.length === 1 ? "" : "s"}.` },
    );
    if (ok) onClear();
  }

  async function onResume() {
    if (resumable.length === 0) return;
    const ok = await withMutationToast(
      () =>
        runChunked(resumable, (c) =>
          pause.mutateAsync({ id: c.id, paused: false }),
        ),
      { success: `Resumed ${resumable.length} connection${resumable.length === 1 ? "" : "s"}.` },
    );
    if (ok) onClear();
  }

  async function onDisconnect() {
    const ok = await withMutationToast(
      () => runChunked(selected, (c) => disconnect.mutateAsync(c.id)),
      { success: `Disconnected ${selected.length} connection${selected.length === 1 ? "" : "s"}.` },
    );
    if (ok) {
      setConfirmOpen(false);
      onClear();
    }
  }

  return (
    <>
      <div className="fixed bottom-6 left-1/2 z-40 -translate-x-1/2">
        <div className="bg-popover text-popover-foreground flex max-w-[calc(100vw-2rem)] items-center gap-1 overflow-hidden rounded-full border p-1 pl-3 shadow-lg">
          <span className="text-sm font-medium whitespace-nowrap">
            {selected.length} selected
          </span>
          <Separator orientation="vertical" className="mx-1 h-5" />

          <Button
            variant="ghost"
            size="sm"
            className="h-8 gap-1.5 rounded-full"
            onClick={onSync}
            disabled={isPending || syncable.length === 0}
            title={
              syncable.length === 0
                ? "No active bank connections selected"
                : `Sync ${syncable.length} active connection${syncable.length === 1 ? "" : "s"}`
            }
          >
            <RefreshCw className="size-4" />
            <span className="hidden sm:inline">Sync</span>
          </Button>

          <Button
            variant="ghost"
            size="sm"
            className="h-8 gap-1.5 rounded-full"
            onClick={onPause}
            disabled={isPending || pausable.length === 0}
            title={
              pausable.length === 0
                ? "No active connections to pause"
                : `Pause ${pausable.length} active connection${pausable.length === 1 ? "" : "s"}`
            }
          >
            <Pause className="size-4" />
            <span className="hidden sm:inline">Pause</span>
          </Button>

          <Button
            variant="ghost"
            size="sm"
            className="h-8 gap-1.5 rounded-full"
            onClick={onResume}
            disabled={isPending || resumable.length === 0}
            title={
              resumable.length === 0
                ? "No paused or errored connections to resume"
                : `Resume ${resumable.length} connection${resumable.length === 1 ? "" : "s"}`
            }
          >
            <Play className="size-4" />
            <span className="hidden sm:inline">Resume</span>
          </Button>

          <Button
            variant="ghost"
            size="sm"
            className="text-destructive hover:bg-destructive/10 hover:text-destructive h-8 gap-1.5 rounded-full"
            onClick={() => setConfirmOpen(true)}
            disabled={isPending || selected.length === 0}
            title={`Disconnect ${selected.length} connection${selected.length === 1 ? "" : "s"}`}
          >
            <Unplug className="size-4" />
            <span className="hidden sm:inline">Disconnect…</span>
          </Button>

          <Separator orientation="vertical" className="mx-1 h-5" />
          <KbdTooltip label="Clear selection" keys={["Esc"]} side="top">
            <Button
              variant="ghost"
              size="icon"
              className="size-8 rounded-full"
              onClick={onClear}
              aria-label="Clear selection"
            >
              <X className="size-4" />
            </Button>
          </KbdTooltip>
        </div>
      </div>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        icon={Unplug}
        title={`Disconnect ${selected.length} connection${selected.length === 1 ? "" : "s"}?`}
        description="Disconnecting wipes saved credentials and soft-deletes the connection's transactions. Accounts stay in the household for historical reference, but no new transactions will sync. This can't be undone."
        confirmLabel={`Disconnect ${selected.length}`}
        pendingLabel="Disconnecting…"
        pending={disconnect.isPending}
        onConfirm={() => void onDisconnect()}
      />
    </>
  );
}
