import { Check, Loader2, RefreshCw, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { EmptyState } from "@/components/empty-state";
import { relativeTime } from "./connection-utils";
import type { SyncLog } from "@/api/queries/sync-logs";

interface SyncHistoryListProps {
  logs: SyncLog[];
}

const STATUS_TILE: Record<string, string> = {
  success: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
  error: "bg-destructive/10 text-destructive",
  in_progress: "bg-primary/10 text-primary",
};

export function SyncHistoryList({ logs }: SyncHistoryListProps) {
  if (logs.length === 0) {
    return (
      <EmptyState
        variant="inline"
        icon={RefreshCw}
        title="No sync history yet"
        description="Each sync run will appear here with its timing and result."
      />
    );
  }

  return (
    <div className="divide-border/40 divide-y">
      {logs.map((log) => (
        <SyncHistoryRow key={log.id} log={log} />
      ))}
    </div>
  );
}

function SyncHistoryRow({ log }: { log: SyncLog }) {
  const ts = log.completed_at ?? log.started_at ?? null;
  const tile = STATUS_TILE[log.status] ?? STATUS_TILE.in_progress;
  const counts = formatCounts(log);
  const errorMessage =
    log.status === "error"
      ? (log.friendly_error_message ?? log.error_message ?? null)
      : null;

  return (
    <div className="flex gap-3 py-3">
      <div className={cn("flex size-6 shrink-0 items-center justify-center rounded-full", tile)}>
        {log.status === "success" ? (
          <Check className="size-3" />
        ) : log.status === "error" ? (
          <X className="size-3" />
        ) : (
          <Loader2 className="size-3 animate-spin" />
        )}
      </div>
      <div className="flex min-w-0 flex-1 items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-medium capitalize">{log.trigger}</span>
            <span className="text-muted-foreground text-xs tabular-nums">
              {ts ? relativeTime(ts) : ""}
            </span>
            {log.duration && (
              <span className="text-muted-foreground bg-muted/60 rounded-full px-1.5 py-0.5 text-[0.6rem] tabular-nums">
                {log.duration}
              </span>
            )}
          </div>
          {errorMessage && (
            <p className="text-destructive/80 mt-0.5 truncate text-xs" title={log.error_message ?? errorMessage}>
              {errorMessage}
            </p>
          )}
        </div>
        {counts && (
          <div className="text-muted-foreground shrink-0 text-xs tabular-nums">
            {counts}
          </div>
        )}
      </div>
    </div>
  );
}

function formatCounts(log: SyncLog): string | null {
  const parts: string[] = [];
  if (log.added_count) parts.push(`${log.added_count} new`);
  if (log.modified_count) parts.push(`${log.modified_count} updated`);
  if (log.removed_count) parts.push(`${log.removed_count} removed`);
  if (parts.length === 0) return null;
  return parts.join(" · ");
}
