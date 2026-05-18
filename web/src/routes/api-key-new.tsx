import { KeyRound } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { SectionCard } from "@/components/section-card";
import { SoftBackButton } from "@/components/soft-back-button";
import { APIKeyForm } from "@/features/api-keys/api-key-form";

export function APIKeyNewPage() {
  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-5">
      <SoftBackButton to="/api-keys">Back to API keys</SoftBackButton>
      <PageHeader
        eyebrow="New credential"
        title="Mint an API key"
        description="A credential for agents, the CLI, or the MCP server. The plaintext shows once after creation — copy it to your password manager before leaving the page."
      />
      <SectionCard
        icon={<KeyRound className="text-muted-foreground size-4" />}
        title="Key details"
      >
        <APIKeyForm />
      </SectionCard>
    </div>
  );
}
