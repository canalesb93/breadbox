import { Loader2 } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { PageError } from "@/components/page-error";
import {
  useProviderConfig,
  useProviderHealth,
} from "@/api/queries/provider-config";
import { CsvCard, PlaidCard, TellerCard } from "@/features/providers";
import type { ProviderConfigResponse, ProviderHealthResponse } from "@/api/types";

// Canonical description — hoisted so every render state (loading, error,
// loaded) reads with the same voice and the page never momentarily swaps
// to a shorter sentence on transition.
const PROVIDERS_DESCRIPTION =
  "Bank data providers that sync accounts and transactions into Breadbox. Each provider stores its own encrypted credentials.";

// Build the small "N healthy · M of 3 configured" eyebrow that lands in
// PageHeader so the page reads as a status overview before the cards.
function buildEyebrow(
  config: ProviderConfigResponse,
  health: Record<string, ProviderHealthResponse>,
): string {
  const configured = [
    config.plaid.configured,
    config.teller.configured,
    true, // CSV is always available
  ].filter(Boolean).length;
  const healthyCount = ["plaid", "teller", "csv"].reduce((acc, key) => {
    const h = health[key];
    if (h && h.last_sync_time && h.last_sync_status === "success") return acc + 1;
    return acc;
  }, 0);
  if (healthyCount > 0) {
    return `${healthyCount} healthy · ${configured} of 3 configured`;
  }
  return `${configured} of 3 configured`;
}

export function ProvidersPage() {
  const config = useProviderConfig();
  const health = useProviderHealth();

  if (config.isLoading) {
    return (
      <>
        <PageHeader
          eyebrow="Settings"
          title="Providers"
          description={PROVIDERS_DESCRIPTION}
        />
        <div className="text-muted-foreground flex items-center justify-center gap-2 py-16 text-sm">
          <Loader2 className="size-4 animate-spin" /> Loading provider configuration…
        </div>
      </>
    );
  }

  if (config.isError || !config.data) {
    return (
      <>
        <PageHeader
          eyebrow="Settings"
          title="Providers"
          description={PROVIDERS_DESCRIPTION}
        />
        <PageError
          resource="provider configuration"
          error={config.error}
          onRetry={() => config.refetch()}
          retrying={config.isFetching}
        />
      </>
    );
  }

  const { plaid, teller, has_encryption_key } = config.data;
  const healthMap = health.data ?? {};
  const eyebrow = buildEyebrow(config.data, healthMap);

  return (
    <>
      <PageHeader
        eyebrow={eyebrow}
        title="Providers"
        description={PROVIDERS_DESCRIPTION}
      />
      <div className="grid gap-5">
        <PlaidCard config={plaid} health={healthMap["plaid"]} />
        <TellerCard
          config={teller}
          health={healthMap["teller"]}
          hasEncryptionKey={has_encryption_key}
        />
        <CsvCard health={healthMap["csv"]} />
      </div>
    </>
  );
}
