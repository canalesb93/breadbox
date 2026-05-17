import { useMemo } from "react";
import {
  AlertCircle,
  Check,
  Loader2,
  RefreshCw,
  type LucideIcon,
} from "lucide-react";
import { EmptyState } from "@/components/empty-state";
import {
  TimelineRail,
  type TimelineRailTone,
} from "@/components/timeline-rail";
import { relativeTime } from "./connection-utils";
import type { SyncLog } from "@/api/queries/sync-logs";

interface SyncHistoryListProps {
  logs: SyncLog[];
}

// Sync-history rows speak the same TimelineRail vocabulary as the
// transaction-detail activity feed (iter 26 + iter 93). Status maps to
// tone so the disc accent reads at a glance — no need to parse the
// status text to find the failed runs.
//   - success     → success    (emerald — run landed cleanly)
//   - error       → destructive(red    — hard failure, demands action)
//   - in_progress → primary    (run is mid-flight; spinner reinforces)
// Iter 105 added the `destructive` tone to TimelineRail specifically so
// errored sync runs don't have to fall back to the softer amber/warning
// tint used for "tag removed" / soft-warning vocabulary.
const STATUS_TONE: Record<string, TimelineRailTone> = {
  success: "success",
  error: "destructive",
  in_progress: "primary",
};

const STATUS_ICON: Record<string, LucideIcon> = {
  success: Check,
  error: AlertCircle,
  in_progress: Loader2,
};

const dayHeadingFormatter = new Intl.DateTimeFormat("en-US", {
  weekday: "long",
  month: "long",
  day: "numeric",
});

function dayLabel(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const today = new Date();
  const yesterday = new Date(today);
  yesterday.setDate(today.getDate() - 1);
  if (d.toDateString() === today.toDateString()) return "Today";
  if (d.toDateString() === yesterday.toDateString()) return "Yesterday";
  return dayHeadingFormatter.format(d);
}

interface DayGroup {
  label: string;
  rows: SyncLog[];
}

// Buckets newest-first sync logs into calendar days, preserving order
// within each day. Logs without any timestamp fall under an "Unknown
// time" group so they don't silently drop. Mirrors the
// activity-timeline `groupByDay` shape so the two feeds read as one
// system.
function groupByDay(logs: SyncLog[]): DayGroup[] {
  const groups: DayGroup[] = [];
  for (const log of logs) {
    const ts = log.completed_at ?? log.started_at ?? null;
    const label = ts ? dayLabel(ts) : "Unknown time";
    const tail = groups[groups.length - 1];
    if (tail && tail.label === label) {
      tail.rows.push(log);
    } else {
      groups.push({ label, rows: [log] });
    }
  }
  return groups;
}

export function SyncHistoryList({ logs }: SyncHistoryListProps) {
  const groups = useMemo(() => groupByDay(logs), [logs]);

  if (logs.length === 0) {
    return (
      <EmptyState
        variant="inline"
        icon={RefreshCw}
        title="No sync history yet"
        description="Every sync run lands here — manual, scheduled, or webhook-triggered — with its timing, counts, and result."
      />
    );
  }

  return (
    <TimelineRail>
      {groups.map((group) => (
        <TimelineRail.Group key={group.label} label={group.label}>
          {group.rows.map((log) => (
            <SyncHistoryRow key={log.id} log={log} />
          ))}
        </TimelineRail.Group>
      ))}
    </TimelineRail>
  );
}

function SyncHistoryRow({ log }: { log: SyncLog }) {
  const ts = log.completed_at ?? log.started_at ?? null;
  const tone = STATUS_TONE[log.status] ?? "neutral";
  const Icon = STATUS_ICON[log.status] ?? RefreshCw;
  const spin = log.status === "in_progress";
  const counts = formatCounts(log);
  const errorMessage =
    log.status === "error"
      ? (log.friendly_error_message ?? log.error_message ?? null)
      : null;

  return (
    <TimelineRail.Row
      icon={Icon}
      tone={tone}
      iconClassName={spin ? "[&>svg]:animate-spin" : undefined}
    >
      <div className="flex min-w-0 flex-1 items-start justify-between gap-3">
        <div className="min-w-0 space-y-0.5">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-medium capitalize">
              {log.trigger}
            </span>
            {log.duration && (
              <span className="text-muted-foreground bg-muted/60 rounded-full px-1.5 py-0.5 text-[10px] tabular-nums">
                {log.duration}
              </span>
            )}
          </div>
          <p className="text-muted-foreground text-xs tabular-nums">
            {ts ? relativeTime(ts) : "Unknown time"}
          </p>
          {errorMessage && (
            <p
              className="text-destructive/80 mt-1 text-xs"
              title={log.error_message ?? errorMessage}
            >
              {errorMessage}
            </p>
          )}
        </div>
        {counts && (
          <div className="text-muted-foreground shrink-0 pt-0.5 text-xs tabular-nums">
            {counts}
          </div>
        )}
      </div>
    </TimelineRail.Row>
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
