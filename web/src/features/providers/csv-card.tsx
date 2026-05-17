import { Link } from "@tanstack/react-router";
import { FileSpreadsheet, Upload } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ColorRailCard } from "@/components/color-rail-card";
import { ProviderCardHeader } from "@/components/provider-card-header";
import { SectionCard } from "@/components/section-card";
import type { ProviderHealthResponse } from "@/api/types";
import {
  ProviderScoreboard,
  providerToneAccent,
  resolveProviderTone,
} from "./provider-status";

interface CsvCardProps {
  health: ProviderHealthResponse | undefined;
}

export function CsvCard({ health }: CsvCardProps) {
  // CSV is always available — there are no credentials to configure. Use the
  // "configured" tone so the rail still encodes "this provider is usable"
  // (warning when never imported, success after a successful import).
  const tone = resolveProviderTone(health, true);

  return (
    <div className="space-y-4">
      <ColorRailCard accent={providerToneAccent(tone)}>
        <ProviderCardHeader
          icon={<FileSpreadsheet className="size-5" />}
          iconClassName="bg-amber-500/10 text-amber-600 dark:text-amber-400"
          title="CSV import"
          description="Drop in transactions exported from any bank — no API credentials required."
          trailing={
            <ProviderScoreboard health={health} tone={tone} alwaysAvailable />
          }
        />
      </ColorRailCard>

      <SectionCard
        title="Import"
        icon={<Upload className="text-muted-foreground size-4" />}
        action={
          <Button asChild size="sm">
            <Link to="/connections" search={{ action: "connect" }}>
              <Upload className="size-3.5" />
              Import CSV
            </Link>
          </Button>
        }
      >
        <p className="text-muted-foreground text-sm">
          Useful when a bank isn't supported by Plaid or Teller, or as a one-time backfill
          for historical transactions. Drag-and-drop a CSV file in the connections wizard,
          map columns, and Breadbox will normalise the data into the same shape as
          API-synced transactions.
        </p>
      </SectionCard>
    </div>
  );
}
