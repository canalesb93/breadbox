import { useMemo } from "react";
import { cn } from "@/lib/utils";
import type { SyncLog } from "@/api/queries/sync-logs";

interface SyncActivityBarsProps {
  logs: SyncLog[];
  /** Number of trailing days to render. Default 7. */
  days?: number;
}

interface DayStats {
  key: string; // YYYY-MM-DD (local)
  shortLabel: string; // M, T, W…
  longLabel: string; // Mon May 5
  success: number;
  error: number;
  total: number;
}

const SHORT_DAY = ["S", "M", "T", "W", "T", "F", "S"];
const LONG_DAY = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

function localKey(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

// Tiny inline bar chart (no charting library). Aggregates the supplied logs
// into the trailing N days, splitting each bar into a green success layer and
// a red error layer scaled to the day's max. Hover to see the day's totals
// via the native title tooltip.
export function SyncActivityBars({ logs, days = 7 }: SyncActivityBarsProps) {
  const buckets: DayStats[] = useMemo(() => {
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const out: DayStats[] = [];
    for (let i = days - 1; i >= 0; i--) {
      const d = new Date(today);
      d.setDate(today.getDate() - i);
      out.push({
        key: localKey(d),
        shortLabel: SHORT_DAY[d.getDay()],
        longLabel: `${LONG_DAY[d.getDay()]} ${d.getMonth() + 1}/${d.getDate()}`,
        success: 0,
        error: 0,
        total: 0,
      });
    }
    const byKey = new Map(out.map((b) => [b.key, b]));
    for (const log of logs) {
      const ts = log.completed_at ?? log.started_at;
      if (!ts) continue;
      const d = new Date(ts);
      if (isNaN(d.getTime())) continue;
      const key = localKey(d);
      const bucket = byKey.get(key);
      if (!bucket) continue;
      bucket.total += 1;
      if (log.status === "success") bucket.success += 1;
      else if (log.status === "error") bucket.error += 1;
    }
    return out;
  }, [logs, days]);

  const maxTotal = Math.max(1, ...buckets.map((b) => b.total));

  return (
    <div className="flex items-end gap-1.5" style={{ height: 72 }}>
      {buckets.map((b) => {
        const pct = b.total === 0 ? 0 : (b.total / maxTotal) * 100;
        const successPct = b.total === 0 ? 0 : (b.success / b.total) * pct;
        const errorPct = b.total === 0 ? 0 : (b.error / b.total) * pct;
        const tooltip =
          b.total === 0
            ? `${b.longLabel} — no syncs`
            : `${b.longLabel} — ${b.total} sync${b.total === 1 ? "" : "s"}` +
              (b.error ? ` (${b.error} err)` : "");
        return (
          <div
            key={b.key}
            className="group relative flex flex-1 flex-col items-center gap-1"
            title={tooltip}
          >
            <div
              className="bg-muted/40 flex w-full flex-col-reverse overflow-hidden rounded-md"
              style={{ height: 52 }}
            >
              {b.success > 0 && (
                <div
                  className={cn(
                    "w-full bg-emerald-500/60 transition-colors group-hover:bg-emerald-500/80",
                  )}
                  style={{ height: `${successPct}%` }}
                />
              )}
              {b.error > 0 && (
                <div
                  className={cn(
                    "w-full bg-destructive/60 transition-colors group-hover:bg-destructive/80",
                  )}
                  style={{ height: `${errorPct}%` }}
                />
              )}
            </div>
            <div className="text-muted-foreground text-[0.6rem] leading-none tabular-nums select-none">
              {b.shortLabel}
            </div>
          </div>
        );
      })}
    </div>
  );
}
