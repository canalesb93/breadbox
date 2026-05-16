import { Loader2 } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  useProviderConfig,
  useProviderHealth,
} from "@/api/queries/provider-config";
import { CsvCard, PlaidCard, TellerCard } from "@/features/providers";

export function ProvidersPage() {
  const config = useProviderConfig();
  const health = useProviderHealth();

  if (config.isLoading) {
    return (
      <>
        <PageHeader
          title="Providers"
          description="Configure bank data providers that sync accounts and transactions."
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
          title="Providers"
          description="Configure bank data providers that sync accounts and transactions."
        />
        <Alert variant="destructive">
          <AlertTitle>Couldn't load provider configuration</AlertTitle>
          <AlertDescription>
            {config.error instanceof Error
              ? config.error.message
              : "Try refreshing the page."}
          </AlertDescription>
        </Alert>
      </>
    );
  }

  const { plaid, teller, has_encryption_key } = config.data;
  const healthMap = health.data ?? {};

  return (
    <>
      <PageHeader
        title="Providers"
        description="Configure bank data providers that sync accounts and transactions into Breadbox."
      />
      <div className="grid gap-4">
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
