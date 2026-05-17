import { AlertTriangle, CheckCircle2, CircleDashed } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Eyebrow } from "@/components/eyebrow";
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

  // Mobile (<sm): scoreboard left-aligns under the identity column so it
  // reads as a continuation of the same column instead of floating right
  // with the eye having to jump across the card. The status pill + score
  // cells flow inline on a single row so the block stays one line tall
  // even with both scoreboard values present. From `sm` up we return to
  // the right-stacked layout that pairs with the hero's `sm:flex-row
  // sm:items-center sm:justify-between`.
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2 sm:flex-col sm:items-end sm:gap-2">
      <span
        className={cn(
          "inline-flex items-center rounded-full border px-2 py-0.5 text-[11px] font-medium",
          tonePill,
        )}
      >
        {headlineLabel}
      </span>
      {health && (
        <div className="flex items-center gap-4 sm:text-right">
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
      <Eyebrow as="div">{label}</Eyebrow>
      <div className="text-foreground text-lg font-semibold tabular-nums">{value}</div>
    </div>
  );
}

