import { Activity, AlertTriangle, CheckCircle2, CircleDashed, Loader2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { ProviderHealthResponse } from "@/api/types";

export type ProviderTone = "success" | "destructive" | "warning" | "muted";

// resolveProviderTone returns the meaning-encoded tone for a provider row.
// Mirrors the connection-detail hero pattern: success when last sync passed,
// destructive on a sync error, warning when configured-but-never-synced,
// muted when not configured at all. Used to drive the ColorRailCard rail
// colour + the inline status badge.
export function resolveProviderTone(
  health: ProviderHealthResponse | undefined,
  configured: boolean,
): ProviderTone {
  if (health?.last_sync_time && health.last_sync_status === "error") {
    return "destructive";
  }
  if (health?.last_sync_time && health.last_sync_status === "success") {
    return "success";
  }
  if (configured) return "warning";
  return "muted";
}

// providerToneAccent returns the CSS colour for the ColorRailCard rail.
// Uses the same fallback pattern as connection-detail: prefer the token,
// fall back to an oklch literal so themes without the token still resolve.
export function providerToneAccent(tone: ProviderTone): string {
  switch (tone) {
    case "success":
      return "var(--success, oklch(0.62 0.18 145))";
    case "destructive":
      return "var(--destructive)";
    case "warning":
      return "var(--warning, oklch(0.78 0.16 75))";
    case "muted":
    default:
      return "var(--muted)";
  }
}

interface ProviderStatusBadgeProps {
  health: ProviderHealthResponse | undefined;
  configured: boolean;
}

// ProviderStatusBadge mirrors the 4-way state surfaced by the v1 provider
// card header — healthy, sync error, configured-not-synced, not configured.
// The colored dot is part of the badge so the row reads at a glance without
// landing on the text first.
export function ProviderStatusBadge({ health, configured }: ProviderStatusBadgeProps) {
  const tone = resolveProviderTone(health, configured);
  if (tone === "destructive") {
    return (
      <Badge variant="outline" className="border-destructive/40 text-destructive">
        <AlertTriangle className="size-3" />
        Sync error
      </Badge>
    );
  }
  if (tone === "success") {
    return (
      <Badge variant="outline" className="border-emerald-500/40 text-emerald-600 dark:text-emerald-400">
        <CheckCircle2 className="size-3" />
        Healthy
      </Badge>
    );
  }
  if (tone === "warning") {
    return (
      <Badge variant="outline" className="border-amber-500/40 text-amber-600 dark:text-amber-400">
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

interface ProviderScoreboardProps {
  health: ProviderHealthResponse | undefined;
  tone: ProviderTone;
  // Optional "always available" override (used by CSV which has no toggle).
  alwaysAvailable?: boolean;
}

// ProviderScoreboard is the right-column metric block on the provider hero.
// Mirrors the ColorRailCard right-column shape used by Connection-detail /
// Account-detail heroes: small uppercase label + tabular value, plus an
// inline status pill in the tone colour. Hides the "connections / accounts"
// scoreboard when health hasn't loaded yet so it doesn't read as zeros.
export function ProviderScoreboard({ health, tone, alwaysAvailable }: ProviderScoreboardProps) {
  const tonePill =
    tone === "success"
      ? "border-emerald-500/40 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10"
      : tone === "destructive"
        ? "border-destructive/40 text-destructive bg-destructive/10"
        : tone === "warning"
          ? "border-amber-500/40 text-amber-600 dark:text-amber-400 bg-amber-500/10"
          : "border-border text-muted-foreground bg-muted/40";
  const headlineLabel = alwaysAvailable
    ? "Always available"
    : tone === "success"
      ? `Synced ${health?.last_sync_time ?? ""}`
      : tone === "destructive"
        ? `Failed ${health?.last_sync_time ?? ""}`
        : tone === "warning"
          ? health?.connection_count
            ? "Never synced"
            : "Awaiting connection"
          : "Not connected";

  return (
    <div className="flex flex-col items-end gap-2 sm:items-end">
      <span
        className={cn(
          "inline-flex items-center rounded-full border px-2 py-0.5 text-[11px] font-medium",
          tonePill,
        )}
      >
        {headlineLabel}
      </span>
      {health && (
        <div className="flex items-center gap-4 text-right">
          <ScoreCell label="Connections" value={String(health.connection_count)} />
          <ScoreCell label="Accounts" value={String(health.account_count)} />
        </div>
      )}
    </div>
  );
}

function ScoreCell({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-muted-foreground text-[10px] font-medium tracking-[0.08em] uppercase">
        {label}
      </div>
      <div className="text-foreground text-lg font-semibold tabular-nums">{value}</div>
    </div>
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
