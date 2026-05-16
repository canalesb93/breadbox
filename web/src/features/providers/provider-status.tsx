import { Activity, AlertTriangle, CheckCircle2, CircleDashed, Loader2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { ProviderHealthResponse } from "@/api/types";

interface ProviderStatusBadgeProps {
  health: ProviderHealthResponse | undefined;
  configured: boolean;
}

// ProviderStatusBadge mirrors the 4-way state surfaced by the v1 provider
// card header — healthy, sync error, configured-not-synced, not configured.
// The colored dot is part of the badge so the row reads at a glance without
// landing on the text first.
export function ProviderStatusBadge({ health, configured }: ProviderStatusBadgeProps) {
  if (health?.last_sync_time && health.last_sync_status === "error") {
    return (
      <Badge variant="outline" className="border-destructive/40 text-destructive">
        <AlertTriangle className="size-3" />
        Sync error
      </Badge>
    );
  }
  if (health?.last_sync_time && health.last_sync_status === "success") {
    return (
      <Badge variant="outline" className="border-emerald-500/40 text-emerald-600 dark:text-emerald-400">
        <CheckCircle2 className="size-3" />
        Healthy
      </Badge>
    );
  }
  if (configured) {
    return (
      <Badge variant="outline" className="border-emerald-500/40 text-emerald-600 dark:text-emerald-400">
        <CheckCircle2 className="size-3" />
        Configured
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="text-muted-foreground">
      <CircleDashed className="size-3" />
      Not configured
    </Badge>
  );
}

interface ProviderStatsProps {
  health: ProviderHealthResponse | undefined;
}

// ProviderStats renders the connections / accounts / last-sync row under the
// card header. Hides itself entirely if there's no data — empty stats look
// like a loading state and confuse users.
export function ProviderStats({ health }: ProviderStatsProps) {
  if (!health) return null;
  const inProgress = health.last_sync_status === "in_progress";
  return (
    <dl className="text-muted-foreground flex flex-wrap items-center gap-x-6 gap-y-1.5 text-xs">
      <Stat label="Connections" value={String(health.connection_count)} />
      <Stat label="Accounts" value={String(health.account_count)} />
      {health.last_sync_time && (
        <div className="flex items-center gap-1.5">
          <Activity className="size-3" />
          <dt>Last sync</dt>
          <dd
            className={cn(
              "text-foreground font-medium",
              health.last_sync_status === "success" && "text-emerald-600 dark:text-emerald-400",
              health.last_sync_status === "error" && "text-destructive",
            )}
          >
            {health.last_sync_status === "error" ? `failed ${health.last_sync_time}` : health.last_sync_time}
          </dd>
        </div>
      )}
      {!health.last_sync_time && health.connection_count > 0 && (
        <div className="flex items-center gap-1.5">
          <Activity className="size-3" />
          <span>Never synced</span>
        </div>
      )}
      {inProgress && (
        <div className="text-foreground flex items-center gap-1.5 font-medium">
          <Loader2 className="size-3 animate-spin" />
          In progress
        </div>
      )}
    </dl>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center gap-1.5">
      <dt>{label}</dt>
      <dd className="text-foreground font-medium">{value}</dd>
    </div>
  );
}
